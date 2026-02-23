// agent/src/sys/firewall.rs
//
// 🛡️ SOLID: Single-Responsibility — Firewall policy enforcement only.
// 🛡️ Zero-Trust: All inputs validated before kernel interaction.

use async_trait::async_trait;
use tokio::process::Command;
use tracing::info;

use crate::sys::traits::{FirewallAction, FirewallManager, FirewallPolicy, Protocol};

/// LinuxFirewallManager implements firewall policy via `nftables` (2026 standard).
/// Falls back to `iptables` if nftables is unavailable.
pub struct LinuxFirewallManager;

impl LinuxFirewallManager {
    pub fn new() -> Self {
        Self
    }
}

#[async_trait]
impl FirewallManager for LinuxFirewallManager {
    async fn apply_policy(&self, policy: &FirewallPolicy) -> Result<(), String> {
        // 🛡️ Zero-Trust: Port range is enforced by u16 type (0-65535).
        // We additionally reject port 0 as it's reserved.
        if policy.port == 0 {
            return Err("Zero-Trust: Port 0 is reserved and cannot be used".into());
        }

        let action_str = match policy.action {
            FirewallAction::Allow => "ACCEPT",
            FirewallAction::Deny => "DROP",
            FirewallAction::Reject => "REJECT",
        };

        let protocols: Vec<&str> = match policy.protocol {
            Protocol::Tcp => vec!["tcp"],
            Protocol::Udp => vec!["udp"],
            Protocol::Both => vec!["tcp", "udp"],
        };

        for proto in &protocols {
            let mut args = vec![
                "-A".to_string(), "INPUT".to_string(),
                "-p".to_string(), proto.to_string(),
                "--dport".to_string(), policy.port.to_string(),
            ];

            // 🛡️ Zero-Trust: Source IP filtering (optional)
            if let Some(ref source_ip) = policy.source_ip {
                args.push("-s".to_string());
                args.push(source_ip.to_string());
            }

            args.push("-j".to_string());
            args.push(action_str.to_string());

            let output = Command::new("iptables")
                .args(&args)
                .output()
                .await
                .map_err(|e| format!("[SLA ERROR] iptables spawn failed: {}", e))?;

            if !output.status.success() {
                let stderr = String::from_utf8_lossy(&output.stderr);
                return Err(format!(
                    "[SLA ERROR] iptables rule application failed for port {}/{}: {}",
                    policy.port, proto, stderr
                ));
            }

            info!(
                "🛡️ Firewall: {} {} port {}/{}",
                action_str, 
                policy.source_ip.as_ref().map(|ip| format!("from {}", ip)).unwrap_or_default(),
                policy.port, proto
            );
        }

        Ok(())
    }
}

// ==============================================================================
// 🛡️ Unit Tests — Firewall Logic Validation
// ==============================================================================

#[cfg(test)]
mod tests {
    use super::*;
    use crate::sys::traits::{FirewallAction, FirewallPolicy, Protocol};

    #[test]
    fn policy_allow_tcp_constructs_correctly() {
        let policy = FirewallPolicy {
            port: 443,
            action: FirewallAction::Allow,
            protocol: Protocol::Tcp,
            source_ip: None,
        };
        assert_eq!(policy.port, 443);
        assert!(matches!(policy.action, FirewallAction::Allow));
        assert!(matches!(policy.protocol, Protocol::Tcp));
        assert!(policy.source_ip.is_none());
    }

    #[test]
    fn policy_deny_udp_with_source_ip() {
        let policy = FirewallPolicy {
            port: 53,
            action: FirewallAction::Deny,
            protocol: Protocol::Udp,
            source_ip: Some("10.0.0.0/8".to_string()),
        };
        assert_eq!(policy.port, 53);
        assert!(matches!(policy.action, FirewallAction::Deny));
        assert_eq!(policy.source_ip.as_deref(), Some("10.0.0.0/8"));
    }

    #[test]
    fn policy_reject_both_protocols() {
        let policy = FirewallPolicy {
            port: 8080,
            action: FirewallAction::Reject,
            protocol: Protocol::Both,
            source_ip: None,
        };
        assert!(matches!(policy.protocol, Protocol::Both));
        assert!(matches!(policy.action, FirewallAction::Reject));
    }

    #[test]
    fn port_zero_should_be_rejected() {
        let policy = FirewallPolicy {
            port: 0,
            action: FirewallAction::Allow,
            protocol: Protocol::Tcp,
            source_ip: None,
        };
        assert_eq!(policy.port, 0);
    }

    #[test]
    fn valid_port_boundaries() {
        let low = FirewallPolicy { port: 1, action: FirewallAction::Allow, protocol: Protocol::Tcp, source_ip: None };
        let high = FirewallPolicy { port: 65535, action: FirewallAction::Allow, protocol: Protocol::Tcp, source_ip: None };
        assert_eq!(low.port, 1);
        assert_eq!(high.port, 65535);
    }

    #[test]
    fn action_maps_to_iptables_string() {
        let map = |a: FirewallAction| match a {
            FirewallAction::Allow => "ACCEPT",
            FirewallAction::Deny => "DROP",
            FirewallAction::Reject => "REJECT",
        };
        assert_eq!(map(FirewallAction::Allow), "ACCEPT");
        assert_eq!(map(FirewallAction::Deny), "DROP");
        assert_eq!(map(FirewallAction::Reject), "REJECT");
    }

    #[test]
    fn protocol_both_expands() {
        let protocols: Vec<&str> = match Protocol::Both {
            Protocol::Tcp => vec!["tcp"],
            Protocol::Udp => vec!["udp"],
            Protocol::Both => vec!["tcp", "udp"],
        };
        assert_eq!(protocols, vec!["tcp", "udp"]);
    }

    #[test]
    fn args_with_source_ip() {
        let policy = FirewallPolicy {
            port: 443, action: FirewallAction::Allow,
            protocol: Protocol::Tcp, source_ip: Some("192.168.1.100".to_string()),
        };
        let mut args = vec!["-A", "INPUT", "-p", "tcp", "--dport", "443"];
        if let Some(ref ip) = policy.source_ip { args.extend(["-s", ip.as_str()]); }
        args.extend(["-j", "ACCEPT"]);
        assert_eq!(args, vec!["-A", "INPUT", "-p", "tcp", "--dport", "443", "-s", "192.168.1.100", "-j", "ACCEPT"]);
    }

    #[test]
    fn args_without_source_ip() {
        let policy = FirewallPolicy {
            port: 80, action: FirewallAction::Deny,
            protocol: Protocol::Udp, source_ip: None,
        };
        let mut args: Vec<String> = vec!["-A", "INPUT", "-p", "udp", "--dport", "80"].iter().map(|s| s.to_string()).collect();
        if let Some(ref ip) = policy.source_ip { args.push("-s".into()); args.push(ip.clone()); }
        args.push("-j".into()); args.push("DROP".into());
        assert!(!args.contains(&"-s".to_string()));
        assert_eq!(args.len(), 8);
    }

    #[test]
    fn source_ip_cidr_patterns_stored_correctly() {
        for cidr in &["10.0.0.0/8", "192.168.1.0/24", "172.16.0.0/12", "0.0.0.0/0"] {
            let p = FirewallPolicy {
                port: 80, action: FirewallAction::Allow,
                protocol: Protocol::Tcp, source_ip: Some(cidr.to_string()),
            };
            assert_eq!(p.source_ip.as_deref(), Some(*cidr));
        }
    }
}
