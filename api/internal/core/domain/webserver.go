package domain
import "context"

type WebServerManager interface {
    ApplyConfig(ctx context.Context, config WebServerConfig) error
    RemoveConfig(ctx context.Context, domainName string) error
}

type WebServerConfig struct {
    DomainName string
    LocalPort  int    // The internal systemd port assigned to the app (e.g., 3000)
    HasSSL     bool   
    AppType    string // e.g., "nodejs", "static"
}
