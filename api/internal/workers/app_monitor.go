package workers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"kari/api/internal/core/domain"
	"math/rand"
)

type AppMonitor struct {
	repo        domain.ApplicationRepository
	auditRepo   domain.AuditRepository
	httpClient  *http.Client
	logger      *slog.Logger
	interval    time.Duration
	concurrency int // 🛡️ SLA: Limit concurrent checks
}

func NewAppMonitor(
	repo domain.ApplicationRepository,
	audit domain.AuditRepository,
	logger *slog.Logger,
	interval time.Duration,
) *AppMonitor {
	return &AppMonitor{
		repo:        repo,
		auditRepo:   audit,
		logger:      logger,
		interval:    interval,
		concurrency: 10, // 🛡️ SLA: Max 10 simultaneous checks
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			// 🛡️ Platform Agnostic: Disable follow-redirects for health checks
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (m *AppMonitor) Start(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.performHealthChecks(ctx)
		}
	}
}

func (m *AppMonitor) performHealthChecks(ctx context.Context) {
	apps, err := m.repo.ListAllActive(ctx)
	if err != nil {
		m.logger.Error("SLA Breach: Failed to list active apps", slog.Any("error", err))
		return
	}

	// 🛡️ SLA: Use a worker pool to bound goroutine creation
	appChan := make(chan domain.Application)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < m.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for app := range appChan {
				// 🛡️ Jitter: Prevent synchronized spikes, but only for active workers
				time.Sleep(time.Duration(rand.Intn(2000)) * time.Millisecond)

				// 🛡️ Per-check Timeout: Don't let one zombie app hang the worker
				checkCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
				m.checkAppHealth(checkCtx, app)
				cancel()
			}
		}()
	}

	// Feed applications to workers
producerLoop:
	for _, app := range apps {
		select {
		case appChan <- app:
		case <-ctx.Done():
			break producerLoop
		}
	}
	close(appChan)
	wg.Wait()
}

func (m *AppMonitor) checkAppHealth(ctx context.Context, app domain.Application) {
	// 🛡️ Platform Agnostic: Allow apps to define custom health paths
	healthPath := app.EnvVars["KARI_HEALTH_PATH"]
	if healthPath == "" {
		healthPath = "/health"
	}

	url := fmt.Sprintf("http://127.0.0.1:%d%s", app.Port, healthPath)

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := m.httpClient.Do(req)

	// A 401/403 might still mean the app is "Running" but the monitor is unauth'd
	// Here we define "Up" as any responsive HTTP listener.
	isUp := err == nil && resp != nil && resp.StatusCode < 500
	if resp != nil {
		resp.Body.Close()
	}

	if !isUp && app.Status == "running" {
		m.handleAppFailure(ctx, app, err)
	} else if isUp && app.Status == "failed" {
		m.handleAppRecovery(ctx, app)
	}
}

func (m *AppMonitor) handleAppFailure(ctx context.Context, app domain.Application, err error) {
	m.logger.Warn("Application health check failed",
		slog.String("id", app.ID.String()),
		slog.Any("error", err),
	)
	_ = m.repo.UpdateStatus(ctx, app.ID, "failed")
}

func (m *AppMonitor) handleAppRecovery(ctx context.Context, app domain.Application) {
	m.logger.Info("Application recovered",
		slog.String("id", app.ID.String()),
	)
	_ = m.repo.UpdateStatus(ctx, app.ID, "running")
}
