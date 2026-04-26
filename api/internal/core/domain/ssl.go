package domain

import (
	"context"
	"time"
)

type SslRepository interface {
	MarkAsSecure(ctx context.Context, domainName string, expiresAt time.Time) error
}
