# Kari development

Install the exact versions in `toolchain.env`, start Docker, and run:

```bash
make verify
```

That command is the complete local and CI contract. Do not substitute isolated
component commands when reporting the repository baseline.

Before requesting commit authorization, generate `kari-source-identity-v2`:

```bash
./scripts/source-identity.sh --manifest .artifacts/source-identity-v2.manifest
```

To regenerate committed protocol artifacts after changing the schema:

```bash
make proto
make verify
```

Generation requires the pinned protoc and Go generator versions. Verification
generates into a temporary directory and never rewrites the working tree.

Rust Clippy treats warnings as errors. `clippy::result-large-err` is the sole
documented allowance because the generated tonic boundary returns `tonic::Status`
by value; changing that public RPC error type is outside this baseline.

For development startup:

```bash
cp .env.example .env
make compose-up
```

The development topology does not launch the native agent. See
[docs/OPERATIONS.md](docs/OPERATIONS.md) for the Linux systemd boundary.
