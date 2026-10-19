-- +goose Up
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    username      TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    is_admin      BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX users_username_lower_idx ON users (lower(username));

CREATE TABLE auth_sessions (
    id           BIGSERIAL PRIMARY KEY,
    token_hash   TEXT        NOT NULL UNIQUE,
    user_id      BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    user_agent   TEXT,
    ip           TEXT
);
CREATE INDEX auth_sessions_user_id_idx ON auth_sessions (user_id);
CREATE INDEX auth_sessions_expires_at_idx ON auth_sessions (expires_at);

CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      JSONB       NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- User-facing job tracking (progress, messages). River owns delivery and retries.
CREATE TABLE jobs (
    id          BIGSERIAL PRIMARY KEY,
    kind        TEXT             NOT NULL,
    state       TEXT             NOT NULL DEFAULT 'queued'
                CHECK (state IN ('queued', 'running', 'done', 'failed')),
    payload     JSONB            NOT NULL DEFAULT '{}',
    progress    DOUBLE PRECISION NOT NULL DEFAULT 0,
    message     TEXT,
    error       TEXT,
    created_at  TIMESTAMPTZ      NOT NULL DEFAULT now(),
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);
CREATE INDEX jobs_created_at_idx ON jobs (created_at DESC);

CREATE TABLE workers (
    id           TEXT PRIMARY KEY,
    hostname     TEXT        NOT NULL,
    version      TEXT        NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE workers;
DROP TABLE jobs;
DROP TABLE settings;
DROP TABLE auth_sessions;
DROP TABLE users;
