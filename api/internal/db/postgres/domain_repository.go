package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/irgordon/kari/api/internal/core/domain"
	"github.com/jmoiron/sqlx"
)

type DomainRepository struct {
	db *sqlx.DB
}

func NewDomainRepository(db *sqlx.DB) *DomainRepository {
	return &DomainRepository{db: db}
}

// Create persists the domain intent and ensures global uniqueness.
func (r *DomainRepository) Create(ctx context.Context, d *domain.Domain) error {
	query := `
		INSERT INTO domains (id, app_id, name, status, target_port, created_at, updated_at)
		VALUES (:id, :app_id, :name, :status, :target_port, :created_at, :updated_at)
	`
	d.ID = uuid.New()
	d.CreatedAt = time.Now()
	d.UpdatedAt = time.Now()

	_, err := r.db.NamedExecContext(ctx, query, d)
	if err != nil {
		// 🛡️ Zero-Trust: Catching unique constraint violations specifically
		return fmt.Errorf("domain already registered or database error: %w", err)
	}
	return nil
}

// GetByAppID retrieves all routing entries for a specific jailed application.
func (r *DomainRepository) GetByAppID(ctx context.Context, appID uuid.UUID) ([]domain.Domain, error) {
	var domains []domain.Domain
	query := `SELECT * FROM domains WHERE app_id = $1 ORDER BY created_at DESC`

	err := r.db.SelectContext(ctx, &domains, query, appID)
	return domains, err
}

// UpdateStatus tracks the transition from 'provisioning' to 'active' or 'failed'.
func (r *DomainRepository) UpdateStatus(ctx context.Context, name string, status string) error {
	query := `UPDATE domains SET status = $1, updated_at = NOW() WHERE name = $2`

	_, err := r.db.ExecContext(ctx, query, status, name)
	return err
}

// Delete removes the domain from the database after a successful Muscle cleanup.
func (r *DomainRepository) Delete(ctx context.Context, name string) error {
	query := `DELETE FROM domains WHERE name = $1`

	_, err := r.db.ExecContext(ctx, query, name)
	return err
}

func (r *DomainRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.Domain, error) {
	var domains []domain.Domain
	query := `SELECT * FROM domains WHERE user_id = $1 ORDER BY created_at DESC`
	err := r.db.SelectContext(ctx, &domains, query, userID)
	return domains, err
}

func (r *DomainRepository) GetByID(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*domain.Domain, error) {
	var domainRecord domain.Domain
	query := `SELECT * FROM domains WHERE id = $1 AND user_id = $2`
	if err := r.db.GetContext(ctx, &domainRecord, query, id, userID); err != nil {
		return nil, err
	}
	return &domainRecord, nil
}

func (r *DomainRepository) GetDomainsWithActiveSSL(ctx context.Context) ([]domain.Domain, error) {
	var domains []domain.Domain
	query := `SELECT * FROM domains WHERE ssl_status = 'active'`
	err := r.db.SelectContext(ctx, &domains, query)
	return domains, err
}

func (r *DomainRepository) FindDueForRenewal(ctx context.Context) ([]domain.Domain, error) {
	var domains []domain.Domain
	query := `SELECT * FROM domains WHERE ssl_status = 'active' AND expires_at <= NOW() + INTERVAL '30 days'`
	err := r.db.SelectContext(ctx, &domains, query)
	return domains, err
}

func (r *DomainRepository) MarkRenewalStatus(ctx context.Context, domainName string, status string) error {
	query := `UPDATE domains SET ssl_status = $1, updated_at = NOW() WHERE domain_name = $2`
	_, err := r.db.ExecContext(ctx, query, status, domainName)
	return err
}
