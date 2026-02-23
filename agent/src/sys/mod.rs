// 🛡️ Zero-Trust Architecture: Modules are private, traits and managers are public.

pub mod traits;     // Global contracts
pub mod secrets;    // Memory hygiene (ProviderCredential)
pub mod proxy;      // Ingress (Nginx/Apache)
pub mod jail;       // User namespacing
pub mod systemd;    // Process jailing
pub mod git;        // Source control
pub mod build;      // Build orchestration
pub mod cleanup;    // Resource hygiene
pub mod ssl;        // Certificate management
pub mod scheduler;  // Cron/Timer scheduling
pub mod logs;       // Log management
pub mod firewall;   // Network policy enforcement

// 🏗️ SLA Re-exports
// We re-export common types so server.rs doesn't have deep nested imports.
