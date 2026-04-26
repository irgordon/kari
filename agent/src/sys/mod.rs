// 🛡️ Zero-Trust Architecture: Modules are private, traits and managers are public.

pub mod build; // Build orchestration
pub mod cleanup; // Resource hygiene
pub mod firewall;
pub mod git; // Source control
pub mod jail; // User namespacing
pub mod logs; // Log management
pub mod proxy; // Ingress (Nginx/Apache)
pub mod scheduler; // Cron/Timer scheduling
pub mod secrets; // Memory hygiene (ProviderCredential)
pub mod ssl; // Certificate management
pub mod systemd; // Process jailing
pub mod traits; // Global contracts // Network policy enforcement

// 🏗️ SLA Re-exports
// We re-export common types so server.rs doesn't have deep nested imports.
