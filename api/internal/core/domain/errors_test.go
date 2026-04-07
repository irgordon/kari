package domain

import (
	"testing"
)

func TestClassifyAgentError(t *testing.T) {
	tests := []struct {
		name     string
		rawError string
		wantCode AgentErrorCode
	}{
		// Resource Limit Exceeded
		{"cgroup error", "process killed by cgroup controller", ErrCgroupLimitExceeded},
		{"OOM error", "Out of memory (OOM)", ErrCgroupLimitExceeded},
		{"memory error", "insufficient memory available", ErrCgroupLimitExceeded},
		{"OOM lowercase", "oom event detected", ErrCgroupLimitExceeded},

		// Application Crashed
		{"exit code error", "process failed with exit code 1", ErrServiceCrashed},
		{"SIGKILL error", "received SIGKILL signal", ErrServiceCrashed},
		{"crashed error", "application crashed during startup", ErrServiceCrashed},

		// Build Failed
		{"build error", "build failed: missing dependencies", ErrBuildFailed},
		{"compile error", "could not compile source code", ErrBuildFailed},
		{"npm error", "npm install failed", ErrBuildFailed},

		// Network Policy Error
		{"iptables error", "failed to update iptables rules", ErrNetworkPolicy},
		{"firewall error", "firewall blocked connection", ErrNetworkPolicy},
		{"network error", "network interface not found", ErrNetworkPolicy},

		// SSL Certificate Error
		{"certificate error", "invalid SSL certificate", ErrCertificateInvalid},
		{"ssl error", "ssl handshake failed", ErrCertificateInvalid},
		{"tls error", "tls connection error", ErrCertificateInvalid},

		// Access Denied
		{"permission error", "permission denied", ErrFilesystemDenied},
		{"access denied error", "access denied to /etc/shadow", ErrFilesystemDenied},
		{"EPERM error", "operation not permitted (EPERM)", ErrFilesystemDenied},

		// Isolation Failure
		{"useradd error", "useradd failed: user already exists", ErrJailProvisionFailed},
		{"systemd-run error", "systemd-run failed to start unit", ErrJailProvisionFailed},
		{"jail error", "failed to enter jail environment", ErrJailProvisionFailed},

		// System Agent Offline
		{"unreachable error", "agent is unreachable", ErrAgentUnreachable},
		{"connection refused error", "connection refused by agent", ErrAgentUnreachable},
		{"socket error", "could not open unix socket", ErrAgentUnreachable},

		// Operation Timed Out
		{"timeout error", "request timeout", ErrServiceTimeout},
		{"deadline error", "context deadline exceeded", ErrServiceTimeout},

		// Internal Error (Default)
		{"unknown error", "something went wrong", ErrUnknown},
		{"empty error", "", ErrUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyAgentError(tt.rawError)
			if got.Code != tt.wantCode {
				t.Errorf("ClassifyAgentError(%q) code = %v, want %v", tt.rawError, got.Code, tt.wantCode)
			}

			// Also verify that Title and Message are not empty for known codes
			if got.Title == "" {
				t.Errorf("ClassifyAgentError(%q) title is empty", tt.rawError)
			}
			if got.Message == "" {
				t.Errorf("ClassifyAgentError(%q) message is empty", tt.rawError)
			}
			if got.Severity == "" {
				t.Errorf("ClassifyAgentError(%q) severity is empty", tt.rawError)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{"exact match", "hello world", "hello", true},
		{"case insensitive match", "Hello World", "hello", true},
		{"substring match", "some long error message", "long error", true},
		{"no match", "hello world", "goodbye", false},
		{"empty string", "", "hello", false},
		{"empty substring", "hello world", "", true},
		{"both empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contains(tt.s, tt.substr); got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}
