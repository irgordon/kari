# 🔌 The Muscle API (gRPC Schema v2.0)

This document defines the interface for the **Karı Muscle** (Rust Agent). It is the source of truth for all cross-language communication.

## 🛡️ Communication Security

* **Transport:** Unix Domain Socket (UDS) located at `/var/run/kari/agent.sock`.
* **Authentication:** **gRPC Peer Credentials**. The Muscle validates the `SO_PEERCRED` of the incoming connection to ensure the caller's UID matches the Brain's designated ID.
* **Privacy:** All `bytes` fields containing sensitive keys are zeroized in the Rust heap immediately after use.

---

## `proto/kari/v1/agent.proto`

```protobuf
syntax = "proto3";

package kari.v1;
option go_package = "kari/api/proto/agent";

// The SystemAgent service manages the physical state of the host.
service SystemAgent {
    // 🛡️ System Integrity & SLA
    // Heartbeat used by the Brain's Healthcheck Prober.
    rpc GetSystemStatus (Empty) returns (SystemStatusResponse);

    // 🛡️ SSL & Let's Encrypt Management
    rpc ManageSslChallenge (ChallengeRequest) returns (ChallengeResponse);
    rpc InstallCertificate (SslInstallRequest) returns (SslInstallResponse);

    // 📦 Jail & Process Orchestration
    rpc ProvisionJail (JailRequest) returns (JailResponse);
    rpc TeardownJail (TeardownRequest) returns (BaseResponse);
    
    // 📡 Live Telemetry
    // Streams build or runtime logs directly from the cgroup buffer.
    rpc StreamLogs (LogRequest) returns (stream LogChunk);
}

// --- Common Messages ---

message Empty {}

message BaseResponse {
    bool success = 1;
    string error_message = 2;
}

// --- System Telemetry ---

message SystemStatusResponse {
    bool healthy = 1;
    uint32 active_jails = 2;
    float cpu_usage_percent = 3;
    float memory_usage_mb = 4;
    string agent_version = 5;
}

// --- SSL Orchestration ---

enum ChallengeAction {
    PRESENT = 0;
    CLEANUP = 1;
}

message ChallengeRequest {
    ChallengeAction action = 1;
    string domain = 2;
    string token = 3;     // HTTP-01 ACME Token
    string key_auth = 4;  // The expected response string
}

message ChallengeResponse {
    bool success = 1;
    string error_message = 2;
}

message SslInstallRequest {
    string domain_name = 1;
    bytes fullchain_pem = 2;
    bytes privkey_pem = 3; // 🛡️ Zero-Trust: Agent must zeroize this!
}

message SslInstallResponse {
    bool success = 1;
    string cert_path = 2;
}

// --- Jail Orchestration ---

message JailRequest {
    string jail_id = 1;
    string repo_url = 2;
    string branch = 3;
    int32 target_port = 4;
    map<string, string> env_vars = 5; // 🛡️ Privacy: Already decrypted by Brain
    
    // Resource Limits (SLA Enforcement)
    uint32 cpu_limit_percent = 6;
    uint32 memory_limit_mb = 7;
}

message JailResponse {
    bool success = 1;
    string jail_id = 2;
    string ipv4_address = 3; // Internal jail IP for proxy routing
}

message TeardownRequest {
    string jail_id = 1;
    bool purge_data = 2; // If true, wipes the jail's persistent dir
}

// --- Log Streaming ---

message LogRequest {
    string trace_id = 1;
    bool follow = 2;     // If true, keeps the stream open (tail -f)
}

message LogChunk {
    bytes data = 1;      // Raw ANSI stream from the process
    string timestamp = 2;
}

```

---

### 🏗️ Key Schema Enhancements

1. **`GetSystemStatus`**: This is critical for the **SLA-grade healthcheck** logic we built. It allows the Brain to report specific resource pressure (CPU/RAM) back to the dashboard.
2. **Resource Limits**: The `JailRequest` now includes `cpu_limit_percent` and `memory_limit_mb`. The Muscle translates these directly into `cgroup v2` parameters (`cpu.max` and `memory.high`).
3. **Map-based Env Vars**: `map<string, string> env_vars` allows for flexible configuration without changing the proto schema every time a new app variable is added.
4. **BaseResponse Inheritance**: Using `BaseResponse` for teardowns and common actions ensures consistent error-handling patterns across both Go and Rust codebases (**SOLID**).

---
