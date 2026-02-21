package adapters

import (
	"context"
	"fmt"

	"kari/api/internal/core/domain"
	"kari/api/internal/grpc/rustagent"
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
	// 🛡️ SLA: Data Translation
	// We convert our Domain Model (ProxyConfig) into the Protobuf format (ProxyRequest).
	req := &rustagent.ProxyRequest{
		Action:     rustagent.ProxyAction_CREATE,
		DomainName: config.DomainName,
		TargetPort: uint32(config.TargetPort), // Protobuf uses uint32 for ports
	}

	resp, err := a.client.ManageProxy(ctx, req)
	if err != nil {
		return fmt.Errorf("gRPC transport error: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("muscle-layer error: %s", resp.ErrorMessage)
	}

	return nil
}

// DeprovisionVHost ensures the remote Apache configuration is purged and the service reloaded.
func (a *ApacheAdapter) DeprovisionVHost(ctx context.Context, domainName string) error {
	req := &rustagent.ProxyRequest{
		Action:     rustagent.ProxyAction_DELETE,
		DomainName: domainName,
	}

	resp, err := a.client.ManageProxy(ctx, req)
	if err != nil {
		return fmt.Errorf("gRPC transport error: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("failed to deprovision vhost: %s", resp.ErrorMessage)
	}

	return nil
}
