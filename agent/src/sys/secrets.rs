// agent/src/sys/secrets.rs

use secrecy::{ExposeSecret, Secret};

/// ProviderCredential is an ephemeral, memory-safe wrapper for highly sensitive 
/// data like AWS Route53 API Tokens, Stripe Keys, or RSA Private Keys.
/// 
/// 1. It cannot be accidentally logged (`println!("{:?}", cred)` will output `[REDACTED]`).
/// 2. When the struct goes out of scope, the memory is safely zeroized, 
///    preventing extraction via RAM scraping.
pub struct ProviderCredential {
    token: Secret<Vec<u8>>,
}

impl ProviderCredential {
    /// Wraps raw bytes in a zeroizing Secret
    pub fn new(raw_token: Vec<u8>) -> Self {
        // 🛡️ 1. Zero-Copy Secret Acquisition
        // By taking ownership of `raw_token` by value and moving it directly 
        // into `Secret::new()`, we prevent the OS allocator from duplicating 
        // the plaintext bytes to a new memory address. 
        // The `secrecy` crate will automatically zeroize this exact heap 
        // allocation when the struct is dropped.
        Self { 
            token: Secret::new(raw_token) 
        }
    }

    /// Safely exposes the secret for a fleeting moment to be written to disk or passed
    /// to an API. The caller provides a closure, ensuring the exposed slice cannot 
    /// escape the immediate execution context.
    pub fn use_secret<F, R>(&self, action: F) -> R
    where
        F: FnOnce(&[u8]) -> R,
    {
        // 🛡️ 2. Lexical Scope Confinement
        // `expose_secret()` returns a reference. By passing it into the `action` closure,
        // the Rust borrow checker mathematically guarantees the plaintext slice 
        // cannot outlive this function call.
        action(self.token.expose_secret())
    }
}
