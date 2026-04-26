package workers

import (
	"context"
	"log/slog"
	"sync"
	"time"

	agent "github.com/irgordon/kari/api/internal/grpc/rustagent"
)

// HealthCache stores the latest system status from the Rust Muscle.
// 🛡️ SLA: Thread-safe via RWMutex for concurrent read access from HTTP handlers.
type HealthCache struct {
	mu       sync.RWMutex
	healthy  bool
	status   *agent.SystemStatus
	lastPing time.Time
}

// HealthProber periodically polls the Rust Muscle's GetSystemStatus RPC
// and updates a global health cache. The Brain reports itself as Unhealthy
// if the Muscle link is severed — enforcing the Fail-Closed design mandate.
type HealthProber struct {
	agent    agent.SystemAgentClient
	cache    *HealthCache
	logger   *slog.Logger
	interval time.Duration
}

// NewHealthProber creates a new background health checker.
// 🛡️ SOLID: Takes the gRPC client interface, not a concrete connection.
func NewHealthProber(agentClient agent.SystemAgentClient, logger *slog.Logger) *HealthProber {
	return &HealthProber{
		agent:    agentClient,
		cache:    &HealthCache{},
		logger:   logger,
		interval: 15 * time.Second,
	}
}

// Start begins the non-blocking polling loop.
func (p *HealthProber) Start(ctx context.Context) {
	p.logger.Info("🩺 Kari Brain: Health Prober started (interval: 15s)")

	// Perform an immediate check on startup
	p.probe(ctx)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("🛑 Kari Brain: Health Prober shutting down...")
			return
		case <-ticker.C:
			p.probe(ctx)
		}
	}
}

// probe executes a single health check against the Muscle.
func (p *HealthProber) probe(ctx context.Context) {
	// 🛡️ SLA: Per-probe timeout prevents a hung Muscle from blocking the Brain
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	status, err := p.agent.GetSystemStatus(probeCtx, &agent.Empty{})
	if err != nil {
		p.logger.Error("[SLA ERROR] Muscle health probe failed",
			slog.Any("error", err),
			slog.Duration("since_last_success", time.Since(p.cache.LastPing())),
		)

		p.cache.mu.Lock()
		p.cache.healthy = false
		p.cache.mu.Unlock()
		return
	}

	p.cache.mu.Lock()
	p.cache.healthy = status.Healthy
	p.cache.status = status
	p.cache.lastPing = time.Now()
	p.cache.mu.Unlock()

	p.logger.Debug("🩺 Muscle heartbeat received",
		slog.Float64("cpu_percent", float64(status.CpuUsagePercent)),
		slog.Float64("memory_mb", float64(status.MemoryUsageMb)),
		slog.Uint64("active_jails", uint64(status.ActiveJails)),
	)
}

// IsHealthy returns true if the last probe succeeded.
// 🛡️ Fail-Closed: Returns false if we've never successfully probed.
func (p *HealthProber) IsHealthy() bool {
	p.cache.mu.RLock()
	defer p.cache.mu.RUnlock()
	return p.cache.healthy
}

// GetStatus returns the latest cached system status (may be nil if never probed).
func (p *HealthProber) GetStatus() *agent.SystemStatus {
	p.cache.mu.RLock()
	defer p.cache.mu.RUnlock()
	return p.cache.status
}

// LastPing returns the time of the last successful probe.
func (c *HealthCache) LastPing() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastPing
}
