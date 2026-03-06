// api/internal/workers/ssl_renewer.go
package workers

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"kari/api/internal/config"
	"kari/api/internal/core/domain"
	"kari/api/internal/core/services"
	"kari/api/internal/core/utils"
)

// ==============================================================================
// 1. Worker Struct (Dependency Injection)
// ==============================================================================

type SSLRenewer struct {
	Config       *config.Config
	DB           domain.DomainRepository
	SSLService   *services.SSLService
	AuditService domain.AuditService
	Logger       *slog.Logger
}

func NewSSLRenewer(
	cfg *config.Config,
	db domain.DomainRepository,
	sslService *services.SSLService,
	auditService domain.AuditService,
	logger *slog.Logger,
) *SSLRenewer {
	return &SSLRenewer{
		Config:       cfg,
		DB:           db,
		SSLService:   sslService,
		AuditService: auditService,
		Logger:       logger,
	}
}

// ==============================================================================
// 2. Lifecycle Management (Graceful Shutdowns)
// ==============================================================================

func (w *SSLRenewer) Start(ctx context.Context) {
	w.Logger.Info("🛡️ SSL Auto-Renewal Worker started")

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	w.checkAndRenew(ctx)

	for {
		select {
		case <-ctx.Done():
			w.Logger.Info("🛑 Shutting down SSL Auto-Renewal Worker gracefully")
			return
		case <-ticker.C:
			w.checkAndRenew(ctx)
		}
	}
}

// ==============================================================================
// 3. Core Worker Logic
// ==============================================================================

func (w *SSLRenewer) checkAndRenew(ctx context.Context) {
	w.Logger.Info("🔍 Running daily SSL expiration check...")

	domains, err := w.DB.GetDomainsWithActiveSSL(ctx)
	if err != nil {
		w.Logger.Error("Failed to fetch domains for SSL check", slog.String("error", err.Error()))
		return
	}

	var renewCount int32
	var failCount int32

	// 🛡️ SLA: Concurrency control via semaphore (max 10 parallel checks)
	// Prevents overwhelming the filesystem or the ACME provider.
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup

	for _, dom := range domains {
		wg.Add(1)
		go func(d domain.Domain) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			// 🛡️ IO-Bound: Reading cert from disk inside the worker pool
			certPath := fmt.Sprintf("%s/%s/fullchain.pem", w.Config.SSLStorageDir, d.DomainName)

			expiresAt, err := utils.GetCertExpiration(certPath)
			if err != nil {
				w.Logger.Warn("Could not parse certificate, skipping",
					slog.String("domain", d.DomainName),
					slog.String("error", err.Error()),
				)
				return
			}

			daysUntilExpiry := time.Until(expiresAt).Hours() / 24

			if daysUntilExpiry <= 30 {
				w.Logger.Info("♻️ Certificate expiring soon, initiating renewal",
					slog.String("domain", d.DomainName),
					slog.Float64("days_left", daysUntilExpiry),
				)

				// 🛡️ Network-Bound: ACME handshake with the provisioner
				err := w.SSLService.ProvisionCertificate(ctx, d.UserID, d.ID)
				if err != nil {
					w.Logger.Error("Failed to renew certificate",
						slog.String("domain", d.DomainName),
						slog.String("error", err.Error()),
					)

					w.AuditService.LogSystemAlert(
						ctx,
						"ssl_renewal_failed",
						"ssl",
						d.ID,
						err,
						"critical",
					)

					atomic.AddInt32(&failCount, 1)
					return
				}

				atomic.AddInt32(&renewCount, 1)
			}
		}(dom)
	}
	wg.Wait()

	if renewCount > 0 || failCount > 0 {
		w.Logger.Info("✅ SSL renewal sweep completed",
			slog.Int("renewed_count", int(renewCount)),
			slog.Int("failed_count", int(failCount)),
		)
	} else {
		w.Logger.Info("✅ SSL renewal sweep completed. No renewals needed today.")
	}
}
