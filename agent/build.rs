fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Re-run the build script if the protobuf definition changes
    println!("cargo:rerun-if-changed=proto/kari/agent/v1/agent.proto");

    tonic_build::configure()
        // The agent is a server only; do not generate client stubs
        .build_client(false)
        .build_server(true)
        .compile(
            &["proto/kari/agent/v1/agent.proto"], // actual proto file
            &["proto"],                           // include root
        )?;

    Ok(())
}
