package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	
	"kari/api/internal/core/domain"
)

var (
	// ErrProfileNotFound is returned when the singleton profile hasn't been initialized.
	ErrProfileNotFound = errors.New("system profile not found")
	
	// ErrConcurrencyConflict is returned when Optimistic Locking detects a race condition.
	ErrConcurrencyConflict = errors.New("optimistic lock failure: the profile was updated by another administrator")
)

// PostgresProfileRepository implements domain.SystemProfileRepository.
// 🛡️ SLA: Wraps the high-performance pgx connection pool.
type PostgresProfileRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresProfileRepository creates a new instance of the repository.
func NewPostgresProfileRepository(pool *pgxpool.Pool) *PostgresProfileRepository {
	return &PostgresProfileRepository{
		pool: pool,
	}
}

// GetActiveProfile fetches the singleton system configuration.
func (r *PostgresProfileRepository) GetActiveProfile(ctx context.Context) (*domain.SystemProfile, error) {
	// 🛡️ Zero-Trust: We limit 1 to enforce the singleton pattern at the query level.
	const query = `
		SELECT 
			id, default_stack_registry, ssl_strategy, max_memory_per_app_mb, 
			max_cpu_percent_per_app, default_firewall_policy, app_user_uid_range_start, 
			app_user_uid_range_end, backup_retention_days, version, updated_at
		FROM system_profiles
		LIMIT 1;
	`

	var p domain.SystemProfile
	
	// Execute the query, respecting the HTTP context timeout
	err := r.pool.QueryRow(ctx, query).Scan(
		&p.ID,
		&p.DefaultStackRegistry, // pgx natively handles mapping JSONB to map[string]string
		&p.SSLStrategy,
		&p.MaxMemoryPerAppMB,
		&p.MaxCPUPercentPerApp,
		&p.DefaultFirewallPolicy,
		&p.AppUserUIDRangeStart,
		&p.AppUserUIDRangeEnd,
		&p.BackupRetentionDays,
		&p.Version,
		&p.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProfileNotFound
		}
		return nil, fmt.Errorf("failed to query active profile: %w", err)
	}

	return &p, nil
}

// UpdateProfile mutates the system state using Optimistic Concurrency Control (OCC).
func (r *PostgresProfileRepository) UpdateProfile(ctx context.Context, profile *domain.SystemProfile) error {
	// 1. 🛡️ Defense-in-Depth: Always validate the domain object before network I/O.
	// If the frontend bypassed UI validation, we catch the garbage data here.
	if err := profile.Validate(); err != nil {
		return fmt.Errorf("invalid profile state: %w", err)
	}

	// 2. 🛡️ Zero-Trust SQL Injection Defense & OCC
	// We use strictly parameterized queries ($1, $2). 
	// The crucial OCC logic is `WHERE id = $1 AND version = $10`. 
	const query = `
		UPDATE system_profiles SET
			default_stack_registry = $2,
			ssl_strategy = $3,
			max_memory_per_app_mb = $4,
			max_cpu_percent_per_app = $5,
			default_firewall_policy = $6,
			app_user_uid_range_start = $7,
			app_user_uid_range_end = $8,
			backup_retention_days = $9,
			version = version + 1,
			updated_at = $11
		WHERE id = $1 AND version = $10;
	`

	now := time.Now().UTC()

	tag, err := r.pool.Exec(ctx, query,
		profile.ID,
		profile.DefaultStackRegistry,
		profile.SSLStrategy,
		profile.MaxMemoryPerAppMB,
		profile.MaxCPUPercentPerApp,
		profile.DefaultFirewallPolicy,
		profile.AppUserUIDRangeStart,
		profile.AppUserUIDRangeEnd,
		profile.BackupRetentionDays,
		profile.Version, // The EXPECTED current version from the client
		now,
	)

	if err != nil {
		return fmt.Errorf("failed to execute profile update: %w", err)
	}

	// 3. 🛡️ Stability: The Optimistic Lock Evaluation
	// If the tag reports 0 rows affected, it means the WHERE clause failed.
	// Either the ID doesn't exist, or the Version in the DB is higher than what 
	// the admin submitted (meaning someone else saved changes first).
	if tag.RowsAffected() == 0 {
		return ErrConcurrencyConflict
	}

	// 4. State Synchronization
	// Mutate the struct in-memory so the caller has the updated state without 
	// needing to perform a secondary SELECT query.
	profile.Version++
	profile.UpdatedAt = now

	return nil
}
