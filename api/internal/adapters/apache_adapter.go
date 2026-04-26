package adapters

import (
	"context"
	"fmt"

	"github.com/irgordon/kari/api/internal/core/domain"
	"github.com/irgordon/kari/api/internal/grpc/rustagent"
)

// ApacheAdapter implements the core.domain.ProxyManager interface.
// It acts as a client to the Rust System Agent.
type ApacheAdapter struct {
	client rustagent.SystemAgentClient
}

// NewApacheAdapter initializes the gRPC-backed proxy manager.
func NewApacheAdapter(client rustagent.SystemAgentClient) *ApacheAdapter {
	return &ApacheAdapter{
		client: client,
	}
}

// ProvisionVHost sends an authenticated request to the Rust Muscle to create an Apache config.
func (a *ApacheAdapter) ProvisionVHost(ctx context.Context, config domain.ProxyConfig) error {
	_ = ctx
	_ = config
	_ = a.client
	return fmt.Errorf("apache proxy management is not supported by the current agent API")
}

// DeprovisionVHost ensures the remote Apache configuration is purged and the service reloaded.
func (a *ApacheAdapter) DeprovisionVHost(ctx context.Context, domainName string) error {
	_ = ctx
	_ = domainName
	_ = a.client
	return fmt.Errorf("apache proxy management is not supported by the current agent API")
}
