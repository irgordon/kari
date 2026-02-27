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
	repo       domain.ApplicationRepository
	auditRepo  domain.AuditRepository
	httpClient *http.Client
	logger     *slog.Logger
	interval   time.Duration
	concurrency int // 🛡️ SLA: Limit concurrent checks
}

func NewAppMonitor(
	repo domain.ApplicationRepository,
	audit domain.AuditRepository,
	logger *slog.Logger,
	interval time.Duration,
) *AppMonitor {
	return &AppMonitor{
		repo:      repo,
		auditRepo: audit,
		logger:    logger,
		interval:  interval,
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

	// 🛡️ SLA: Concurrency control via semaphore
	sem := make(chan struct{}, m.concurrency)
	var wg sync.WaitGroup

	for _, app := range apps {
		wg.Add(1)

		go func(a domain.Application) {
			defer wg.Done()

			// 🛡️ Jitter: Prevent synchronized spikes
			time.Sleep(time.Duration(rand.Intn(2000)) * time.Millisecond)

			sem <- struct{}{} // Acquire
			defer func() { <-sem }() // Release

			// 🛡️ Per-check Timeout: Don't let one zombie app hang the worker
			checkCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
			defer cancel()
			
			m.checkAppHealth(checkCtx, a)
		}(app)
	}
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

// ... handleAppFailure and handleAppRecovery remain similar but use structured logging ...
