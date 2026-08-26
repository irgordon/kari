# QUICKSTART: Local to Production (Safe Defaults)

This is the shortest path to run Karı with deterministic behavior and minimal setup.

## Safe Defaults Philosophy

- You can run Karı without custom config for normal local development.
- You only need protobuf tooling when changing `.proto` contracts.
- The default workflow is intentionally single-command (`make dev`).
- Advanced knobs are optional and should not block first use.

## 5 Commands

1. Clone repo
```bash
git clone https://github.com/irgordon/kari.git && cd kari
```

2. First-run bootstrap
```bash
./scripts/bootstrap.sh
```

3. Start stack
```bash
make up
```

4. Stream logs
```bash
make logs
```

5. Stop stack
```bash
make down
```

## Example Run (Validation)

```bash
make dev
```

Expected: single pass/fail surface for env normalization, optional proto validation, tests, and deterministic drift checks.

## Example Failure

If Go dependencies are not available in your environment, `make dev` fails during `go test ./...`.
That failure is expected and explicit; resolve module/network constraints, then rerun.
