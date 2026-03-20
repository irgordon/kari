package domain

import "strings"

// AgentErrorCode maps raw gRPC error messages from the Rust Muscle
// into human-readable error codes that the Svelte UI can present
// as styled alerts. This prevents exposing raw system internals to tenants.
//
// 🛡️ Zero-Trust: No internal paths, kernel versions, or system details leak to the browser.
// 🛡️ SLA: Each error has a recommended severity for UI rendering.
type AgentErrorCode string

const (
	ErrCgroupLimitExceeded AgentErrorCode = "RESOURCE_LIMIT_EXCEEDED"
	ErrServiceCrashed      AgentErrorCode = "SERVICE_CRASHED"
	ErrServiceTimeout      AgentErrorCode = "SERVICE_TIMEOUT"
	ErrBuildFailed         AgentErrorCode = "BUILD_FAILED"
	ErrNetworkPolicy       AgentErrorCode = "NETWORK_POLICY_FAILED"
	ErrCertificateInvalid  AgentErrorCode = "CERTIFICATE_INVALID"
	ErrFilesystemDenied    AgentErrorCode = "FILESYSTEM_ACCESS_DENIED"
	ErrJailProvisionFailed AgentErrorCode = "JAIL_PROVISION_FAILED"
	ErrAgentUnreachable    AgentErrorCode = "AGENT_UNREACHABLE"
	ErrUnknown             AgentErrorCode = "INTERNAL_ERROR"
)

// AgentError is a structured error from the Muscle that can be serialized to JSON
// and consumed by the Svelte frontend for user-facing alert rendering.
type AgentError struct {
	Code     AgentErrorCode `json:"code"`
	Title    string         `json:"title"`
	Message  string         `json:"message"`
	Severity string         `json:"severity"` // "critical", "warning", "info"
}

// ClassifyAgentError transforms a raw gRPC error string from the Rust Muscle
// into a structured, UI-safe error. The raw message is logged server-side
// but NEVER sent to the browser.
func ClassifyAgentError(rawError string) AgentError {
	// 🛡️ Pattern matching against known Muscle error prefixes
	switch {
	// Cgroup v2 OOM or CPU throttle
	case contains(rawError, "cgroup") || contains(rawError, "OOM") || contains(rawError, "memory"):
		return AgentError{
			Code:     ErrCgroupLimitExceeded,
			Title:    "Resource Limit Exceeded",
			Message:  "Your application exceeded its allocated CPU or memory. Consider increasing the resource limits in your app settings.",
			Severity: "critical",
		}

	// Systemd service crash
	case contains(rawError, "exit code") || contains(rawError, "SIGKILL") || contains(rawError, "crashed"):
		return AgentError{
			Code:     ErrServiceCrashed,
			Title:    "Application Crashed",
			Message:  "Your application process exited unexpectedly. Check the deployment logs for stack traces or runtime errors.",
			Severity: "critical",
		}

	// Build failure
	case contains(rawError, "build") || contains(rawError, "compile") || contains(rawError, "npm"):
		return AgentError{
			Code:     ErrBuildFailed,
			Title:    "Build Failed",
			Message:  "The build command returned an error. Review the deployment terminal output for the exact failure.",
			Severity: "warning",
		}

	// Firewall / network policy
	case contains(rawError, "iptables") || contains(rawError, "firewall") || contains(rawError, "network"):
		return AgentError{
			Code:     ErrNetworkPolicy,
			Title:    "Network Policy Error",
			Message:  "Failed to apply network rules for your application. Contact your administrator.",
			Severity: "warning",
		}

	// SSL certificate issues
	case contains(rawError, "certificate") || contains(rawError, "ssl") || contains(rawError, "tls"):
		return AgentError{
			Code:     ErrCertificateInvalid,
			Title:    "SSL Certificate Error",
			Message:  "Failed to install or validate the SSL certificate. Ensure your domain's DNS is correctly configured.",
			Severity: "warning",
		}

	// Filesystem permission denied
	case contains(rawError, "permission") || contains(rawError, "access denied") || contains(rawError, "EPERM"):
		return AgentError{
			Code:     ErrFilesystemDenied,
			Title:    "Access Denied",
			Message:  "The system agent was denied access to a required file or directory. This may indicate a configuration issue.",
			Severity: "critical",
		}

	// Jail provisioning failure
	case contains(rawError, "useradd") || contains(rawError, "systemd-run") || contains(rawError, "jail"):
		return AgentError{
			Code:     ErrJailProvisionFailed,
			Title:    "Isolation Failure",
			Message:  "Failed to create the secure application jail. The system may be at capacity. Contact your administrator.",
			Severity: "critical",
		}

	// Agent connectivity
	case contains(rawError, "unreachable") || contains(rawError, "connection refused") || contains(rawError, "socket"):
		return AgentError{
			Code:     ErrAgentUnreachable,
			Title:    "System Agent Offline",
			Message:  "The infrastructure agent is not responding. The system may be restarting. Try again in a few moments.",
			Severity: "critical",
		}

	// Timeout
	case contains(rawError, "timeout") || contains(rawError, "deadline"):
		return AgentError{
			Code:     ErrServiceTimeout,
			Title:    "Operation Timed Out",
			Message:  "The operation took too long and was cancelled. This may indicate high system load.",
			Severity: "warning",
		}

	default:
		return AgentError{
			Code:     ErrUnknown,
			Title:    "Internal Error",
			Message:  "An unexpected error occurred. The system administrator has been notified.",
			Severity: "warning",
		}
	}
}

// contains is a case-insensitive substring check.
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
