# MeshManager

A self-hosted library for your 3D print files (STL, 3MF, OBJ, STEP and more). It indexes the folders where your models already live, renders thumbnails, and lets you search, preview and deduplicate them. It never moves or modifies your files.

> **Status: milestone 0.** The skeleton is in place: the Go API, Postgres with migrations, the background worker, the React shell and single-admin login. Library features come next. See [PLAN.md](PLAN.md).

## Run with Docker

```bash
cp .env.example .env        # optional: set POSTGRES_PASSWORD, PUID/PGID, PORT
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
| `app` | `meshmanager serve`: runs migrations, serves the API and the embedded web UI on :8080 |
| `worker` | `meshmanager worker`: runs background jobs (scanning, thumbnails, …) from the River queue in Postgres. Scale with `docker compose up -d --scale worker=3`. |
| `db` | PostgreSQL 16 with pgvector |

No Redis: the job queue ([River](https://riverqueue.com)) lives in Postgres.

### Configuration

| Variable | Default | |
|---|---|---|
| `DATABASE_URL` | `postgres://mesh:mesh@db:5432/mesh?sslmode=disable` | |
| `LISTEN_ADDR` | `:8080` | |
| `DATA_DIR` | `/data` | thumbnails, caches, backups (later milestones) |
| `ADMIN_USERNAME` / `ADMIN_PASSWORD` | unset | create the admin on first start |
| `SESSION_TTL_DAYS` | `30` | how long a sign-in lasts |
| `TRUST_PROXY` | `false` | trust `X-Forwarded-For`/`-Proto`; enable only behind a reverse proxy |
| `COOKIE_SECURE` | `auto` | `auto` sets the cookie's Secure flag when the request came in over HTTPS; or `true`/`false` |
| `WORKER_CONCURRENCY` | `4` | parallel jobs per worker |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

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

# 2. API (terminal 1) and worker (terminal 2)
cd backend
export DATABASE_URL="postgres://mesh:mesh@localhost:5432/mesh?sslmode=disable" DATA_DIR=./.data
go run ./cmd/meshmanager serve
go run ./cmd/meshmanager worker

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
