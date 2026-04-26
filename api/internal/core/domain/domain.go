package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Domain struct {
	ID           uuid.UUID `json:"id"`
	UserID       uuid.UUID `json:"user_id"`
	AppID        uuid.UUID `json:"app_id"`
	Name         string    `json:"name"`
	DomainName   string    `json:"domain_name"`
	DocumentRoot string    `json:"document_root"`
	SSLStatus    string    `json:"ssl_status"`
	Status       string    `json:"status"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type DomainRepository interface {
	Create(ctx context.Context, domain *Domain) error
	ListByUser(ctx context.Context, userID uuid.UUID) ([]Domain, error)
	GetByID(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*Domain, error)
	UpdateStatus(ctx context.Context, domainName string, status string) error
	Delete(ctx context.Context, domainName string) error
	GetDomainsWithActiveSSL(ctx context.Context) ([]Domain, error)
	FindDueForRenewal(ctx context.Context) ([]Domain, error)
	MarkRenewalStatus(ctx context.Context, domainName string, status string) error
}

type DomainService interface {
	ListDomains(ctx context.Context, userID uuid.UUID) ([]Domain, error)
	CreateDomain(ctx context.Context, domain *Domain) (*Domain, error)
	DeleteDomain(ctx context.Context, domainID uuid.UUID, userID uuid.UUID) error
	TriggerSSLProvisioning(ctx context.Context, domainID uuid.UUID, userID uuid.UUID) error
}
