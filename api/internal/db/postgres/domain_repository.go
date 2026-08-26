package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/irgordon/kari/api/internal/core/domain"
)

type DomainRepository struct {
	pool *pgxpool.Pool
}

func NewDomainRepository(pool *pgxpool.Pool) *DomainRepository {
	return &DomainRepository{pool: pool}
}

func (r *DomainRepository) Create(ctx context.Context, record *domain.Domain) error {
	const query = `
		INSERT INTO domains (
			user_id, app_id, domain_name, document_root, ssl_status, status, expires_at
		) VALUES ($1, NULLIF($2::uuid, $3::uuid), $4, $5, $6, $7, $8)
		RETURNING id, name, created_at, updated_at`
	err := r.pool.QueryRow(ctx, query,
		record.UserID, record.AppID, uuid.Nil, domainName(record), record.DocumentRoot,
		record.SSLStatus, record.Status, record.ExpiresAt,
	).Scan(&record.ID, &record.Name, &record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create domain: %w", err)
	}
	record.DomainName = record.Name
	return nil
}

func (r *DomainRepository) GetByAppID(ctx context.Context, appID uuid.UUID) ([]domain.Domain, error) {
	return r.list(ctx, domainSelect+` WHERE app_id = $1 ORDER BY created_at DESC`, appID)
}

func (r *DomainRepository) UpdateStatus(ctx context.Context, domainName string, status string) error {
	return r.update(ctx, "UPDATE domains SET status = $1 WHERE domain_name = $2", status, domainName)
}

func (r *DomainRepository) Delete(ctx context.Context, domainName string) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM domains WHERE domain_name = $1", domainName)
	if err != nil {
		return fmt.Errorf("delete domain: %w", err)
	}
	return requireAffectedRow(tag.RowsAffected())
}

func (r *DomainRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]domain.Domain, error) {
	return r.list(ctx, domainSelect+` WHERE user_id = $1 ORDER BY created_at DESC`, userID)
}

func (r *DomainRepository) GetByID(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*domain.Domain, error) {
	record, err := scanDomain(r.pool.QueryRow(ctx, domainSelect+` WHERE id = $1 AND user_id = $2`, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get domain: %w", err)
	}
	return record, nil
}

func (r *DomainRepository) GetDomainsWithActiveSSL(ctx context.Context) ([]domain.Domain, error) {
	return r.list(ctx, domainSelect+` WHERE ssl_status = 'active' ORDER BY created_at`)
}

func (r *DomainRepository) FindDueForRenewal(ctx context.Context) ([]domain.Domain, error) {
	const predicate = ` WHERE ssl_status = 'active' AND expires_at <= NOW() + INTERVAL '30 days' ORDER BY expires_at`
	return r.list(ctx, domainSelect+predicate)
}

func (r *DomainRepository) MarkRenewalStatus(ctx context.Context, domainName string, status string) error {
	return r.update(ctx, "UPDATE domains SET ssl_status = $1 WHERE domain_name = $2", status, domainName)
}

func (r *DomainRepository) list(ctx context.Context, query string, args ...any) ([]domain.Domain, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}
	defer rows.Close()
	return collectDomains(rows)
}

func (r *DomainRepository) update(ctx context.Context, query string, status string, name string) error {
	tag, err := r.pool.Exec(ctx, query, status, name)
	if err != nil {
		return fmt.Errorf("update domain: %w", err)
	}
	return requireAffectedRow(tag.RowsAffected())
}

const domainSelect = `
	SELECT id, user_id, COALESCE(app_id, '00000000-0000-0000-0000-000000000000'::uuid),
	       name, domain_name, document_root, ssl_status, status, expires_at, created_at, updated_at
	FROM domains`

type domainRow interface {
	Scan(dest ...any) error
}

func scanDomain(row domainRow) (*domain.Domain, error) {
	var record domain.Domain
	err := row.Scan(
		&record.ID, &record.UserID, &record.AppID, &record.Name, &record.DomainName,
		&record.DocumentRoot, &record.SSLStatus, &record.Status, &record.ExpiresAt,
		&record.CreatedAt, &record.UpdatedAt,
	)
	return &record, err
}

func collectDomains(rows pgx.Rows) ([]domain.Domain, error) {
	domains := make([]domain.Domain, 0)
	for rows.Next() {
		record, err := scanDomain(rows)
		if err != nil {
			return nil, fmt.Errorf("scan domain: %w", err)
		}
		domains = append(domains, *record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate domains: %w", err)
	}
	return domains, nil
}

func domainName(record *domain.Domain) string {
	if record.DomainName != "" {
		return record.DomainName
	}
	return record.Name
}
