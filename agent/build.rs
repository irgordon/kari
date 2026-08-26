fn main() -> Result<(), Box<dyn std::error::Error>> {
    const PROTO_FILE: &str = "../proto/kari/agent/v1/agent.proto";
    const PROTO_ROOT: &str = "../proto";

    println!("cargo:rerun-if-changed={PROTO_FILE}");

    tonic_build::configure()
        .build_client(false)
        .build_server(true)
        .compile(&[PROTO_FILE], &[PROTO_ROOT])?;

    Ok(())
}
