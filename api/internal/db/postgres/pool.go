package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool initializes a new PostgreSQL connection pool using pgxpool.
// 🛡️ SLA: Configures explicit pooling limits to prevent socket exhaustion during load spikes.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse database url: %w", err)
	}

	// 🛡️ SLA Performance: Pooling thresholds
	config.MaxConns = 50                      // Maximum open connections
	config.MinConns = 5                       // Minimum idle connections kept alive
	config.MaxConnLifetime = time.Hour        // Recycle connections every hour
	config.MaxConnIdleTime = time.Minute * 30 // Close idle connections after 30 mins

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	// 🛡️ Zero-Trust: Verify connectivity immediately
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	return pool, nil
}
