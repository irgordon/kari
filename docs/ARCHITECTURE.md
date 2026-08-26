# Kari baseline architecture

Kari has four runtime components with one supported deployment boundary.

| Component | Runtime | Boundary |
| --- | --- | --- |
| PostgreSQL | Container | Persistent database volume on the internal backplane |
| Go API | Container, UID/GID 1001 | HTTP gateway, database access, native-agent Unix socket |
| React frontend | Nginx container | Static UI and `/api/` reverse proxy |
| Rust agent | Native Linux systemd service as root | Host users, services, firewall, proxy, packages, and files |

`docker-compose.yml` is the container topology authority. The native unit at
`deploy/systemd/kari-agent.service` is the privileged-agent authority. The
shared socket is `/run/kari/agent.sock`; production Compose binds `/run/kari`
into the API container and requires the agent for readiness.

The API exposes `/health` for process liveness and `/ready` for dependency
readiness. Development and CI may set `KARI_REQUIRE_AGENT=false`; that mode is a
database/API/frontend smoke test and is not an end-to-end agent test.

The canonical database chain is listed in
`api/internal/db/migrations/manifest.txt`. The migration binary applies every
pending file in one transaction and records its SHA-256 checksum in
`schema_migrations`.
