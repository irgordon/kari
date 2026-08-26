# Contributing to Kari

Feature work is frozen until the recovery phases are complete. Keep changes
limited to the approved recovery scope and avoid unrelated dependency upgrades.

Before proposing a change, run the repository contract:

```bash
make verify
```

Do not disable tests, ignore required failures, update generated protobuf files
without their source, rewrite applied migrations, or add a second build or
deployment path. Pull requests must pass the required Kari Verify and CodeQL
checks and resolve review conversations before merge.
