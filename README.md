# MeshManager

A self-hosted library for your 3D print files (STL, 3MF, OBJ, STEP and more). It indexes the folders where your models already live, renders thumbnails, and lets you search, preview and deduplicate them. It never moves or modifies your files.

> **Status: milestone 1.** Sources, the background scanner with BLAKE3 hashing, thumbnails (STL, OBJ, PLY, 3MF, GLB and images; slicer previews embedded in 3MF files are used first), and a searchable, virtualized library grid with a folder tree. The 3D viewer comes next. See [PLAN.md](PLAN.md).

## Run with Docker

```bash
cp .env.example .env        # optional: set POSTGRES_PASSWORD, PUID/PGID, PORT
mkdir -p data               # so the folder is owned by you, not root
docker compose up -d --build
```

Open http://localhost:8080. On first visit it asks you to create the admin account. Alternatively, set `ADMIN_USERNAME`/`ADMIN_PASSWORD` in `.env` before the first start.

Mount your model folders read-only in `docker-compose.yml`, under the `x-app` volumes:

```yaml
- /mnt/nas/3d-prints:/libraries/nas:ro
```

### Services

| Service | What it does |
|---|---|
| `app` | `meshmanager serve`: runs migrations, serves the API and web UI on :8080, and runs the background jobs (scanning, hashing, thumbnails) |
| `db` | PostgreSQL 16 with pgvector; also holds the job queue ([River](https://riverqueue.com)), so there's no Redis |
| `worker` | Optional, off by default. Extra job capacity for very large libraries: `docker compose --profile workers up -d --scale worker=2` |

### Settings

Library behaviour is configured in the web UI under **Settings → Library** and saved to `data/config.json`. Changes apply right away, with no restart. You can also edit the file by hand. Missing keys use the defaults, and an invalid file is ignored in favour of the last good values.

| Setting | Default | |
|---|---|---|
| `thumb_size` | `384` | thumbnail edge length in pixels (128–1024); applies to new thumbnails |
| `scan_interval_hours` | `12` | rescan every enabled source this often; `0` = only when you click **Scan now** |
| `hash_concurrency` | `4` | files hashed in parallel during a scan; lower it for slow NAS drives |
| `index_videos` | `false` | also index video files |

### Environment variables (deployment only)

| Variable | Default | |
|---|---|---|
| `DATABASE_URL` | `postgres://mesh:mesh@db:5432/mesh?sslmode=disable` | |
| `LISTEN_ADDR` | `:8080` | |
| `DATA_DIR` | `/data` | `config.json`, thumbnails (`thumbs/<source>/…`), later caches and backups |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | unset | create the admin on first start |
| `SESSION_TTL_DAYS` | `30` | how long a sign-in lasts |
| `TRUST_PROXY` | `false` | trust `X-Forwarded-For`/`-Proto`; enable only behind a reverse proxy |
| `COOKIE_SECURE` | `auto` | `auto` sets the cookie's Secure flag when the request came in over HTTPS; or `true`/`false` |
| `WORKER_CONCURRENCY` | `4` | parallel background jobs (mostly thumbnail rendering) per container |
| `RUN_WORKER` | `true` | set `false` on `app` only if dedicated `worker` containers should do all job work |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LIBRARY_ROOT` | `/libraries` | sources must be folders under this path (where library volumes are mounted) |

### Reverse proxy

Put Caddy, Traefik or Nginx Proxy Manager in front for HTTPS and set `TRUST_PROXY=true`. With Caddy:

```
models.example.com {
    reverse_proxy meshmanager-app-1:8080
}
```

### Locked out?

```bash
docker compose exec -it app meshmanager reset-password -username admin
```

## Development

Requirements: Go 1.27+, Node 24+, and Docker (only for Postgres).

```bash
# 1. Postgres on localhost:5432
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d db

# 2. API and background jobs
cd backend
export DATABASE_URL="postgres://mesh:mesh@localhost:5432/mesh?sslmode=disable" DATA_DIR=./.data LIBRARY_ROOT=/path/to/your/models
go run ./cmd/meshmanager serve

# 3. Frontend with hot reload on http://localhost:5173 (proxies /api to :8080)
cd frontend && npm install && npm run dev
```

### Tests

```bash
cd backend && go test ./...
```

The API tests need Postgres. They use `TEST_DATABASE_URL` when set (CI does this), and otherwise download and start an embedded PostgreSQL 16 automatically, so `go test` works without Docker.

### Layout

```
backend/
  cmd/meshmanager/     entry point: serve, worker, migrate, reset-password, healthcheck
  internal/api/        HTTP handlers, auth middleware, CSRF guard
  internal/auth/       argon2id hashing, session tokens, rate limiter
  internal/config/     environment configuration
  internal/db/         connection, goose migrations (db/migrations/*.sql), River migrations
  internal/jobs/       River job definitions and workers
  internal/settings/   user-editable settings in DATA_DIR/config.json
  internal/store/      SQL queries
  internal/web/        serves the React build embedded in the binary
frontend/              React + TypeScript + Vite + Tailwind
Dockerfile             node build → go build → distroless image (amd64 + arm64)
```

### Security notes

- Passwords are hashed with argon2id. Sessions are random tokens in an HttpOnly, SameSite=Lax cookie, and only a SHA-256 of each token is stored.
- State-changing API calls require an `X-Requested-With` header (CSRF protection).
- Sign-in is limited to 10 failed attempts per IP per 5 minutes.
- Changing your password signs out every other session. Expired sessions are cleaned up hourly by the worker.
