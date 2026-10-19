-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- App settings live in DATA_DIR/config.json instead (see internal/settings).
DROP TABLE settings;

CREATE TABLE sources (
    id                    BIGSERIAL PRIMARY KEY,
    name                  TEXT        NOT NULL,
    path                  TEXT        NOT NULL UNIQUE,
    thumbnail_mode        TEXT        NOT NULL DEFAULT 'lazy'
                          CHECK (thumbnail_mode IN ('lazy', 'eager')),
    enabled               BOOLEAN     NOT NULL DEFAULT TRUE,
    -- Incremented per scan; files not seen in the latest generation are marked missing.
    scan_generation       BIGINT      NOT NULL DEFAULT 0,
    last_scan_job_id      BIGINT REFERENCES jobs (id) ON DELETE SET NULL,
    last_scan_finished_at TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE files (
    id              BIGSERIAL PRIMARY KEY,
    source_id       BIGINT      NOT NULL REFERENCES sources (id) ON DELETE CASCADE,
    rel_path        TEXT        NOT NULL, -- forward slashes, relative to the source root
    dir             TEXT        NOT NULL, -- rel_path's directory, '' at the root
    name            TEXT        NOT NULL,
    kind            TEXT        NOT NULL,
    format          TEXT        NOT NULL,
    size            BIGINT      NOT NULL,
    mtime           TIMESTAMPTZ NOT NULL,
    content_hash    TEXT,
    status          TEXT        NOT NULL DEFAULT 'present'
                    CHECK (status IN ('present', 'missing')),
    seen_generation BIGINT      NOT NULL,
    first_seen_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    missing_since   TIMESTAMPTZ,
    UNIQUE (source_id, rel_path)
);
CREATE INDEX files_source_dir_idx ON files (source_id, dir text_pattern_ops);
CREATE INDEX files_name_idx ON files (lower(name), id);
CREATE INDEX files_mtime_idx ON files (mtime DESC, id DESC);
CREATE INDEX files_size_idx ON files (size DESC, id DESC);
CREATE INDEX files_kind_format_idx ON files (kind, format);
CREATE INDEX files_path_trgm_idx ON files USING gin (rel_path gin_trgm_ops);
CREATE INDEX files_content_hash_idx ON files (content_hash) WHERE content_hash IS NOT NULL;

CREATE TABLE thumbnails (
    file_id          BIGINT PRIMARY KEY REFERENCES files (id) ON DELETE CASCADE,
    status           TEXT        NOT NULL CHECK (status IN ('ready', 'failed')),
    path             TEXT, -- relative to DATA_DIR
    content_type     TEXT,
    width            INT,
    height           INT,
    origin           TEXT, -- rendered | embedded | image
    error            TEXT,
    renderer_version INT         NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE model_info (
    file_id     BIGINT PRIMARY KEY REFERENCES files (id) ON DELETE CASCADE,
    triangles   BIGINT           NOT NULL,
    size_x      DOUBLE PRECISION NOT NULL,
    size_y      DOUBLE PRECISION NOT NULL,
    size_z      DOUBLE PRECISION NOT NULL,
    volume_mm3  DOUBLE PRECISION NOT NULL,
    area_mm2    DOUBLE PRECISION NOT NULL,
    analyzed_at TIMESTAMPTZ      NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE model_info;
DROP TABLE thumbnails;
DROP TABLE files;
DROP TABLE sources;
CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      JSONB       NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
