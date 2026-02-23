fn main() -> Result<(), Box<dyn std::error::Error>> {
    // 🛡️ SLA: Protocol Buffer Compilation
    // This script tells Cargo to re-run if the .proto file changes.
    // It maps our shared Kari protobuf definition into the 'kari_agent' module.
    
    println!("cargo:rerun-if-changed=../proto/kari/agent/v1/agent.proto");

    tonic_build::configure()
        // 🛡️ Zero-Trust: We don't generate client code here because the Agent 
        // is strictly a SERVER. This reduces the final binary attack surface.
        .build_client(false)
        .build_server(true)
        // Ensure we support the LogChunk streaming requirements
        .compile(
            &["../proto/kari/agent/v1/agent.proto"], // Path to the shared definition
            &["../proto"],                           // Include paths for imports
        )?;

    Ok(())
}
