-- +goose Up

-- Tags. The colour is a palette key; the frontend turns it into a colour that suits
-- the light or dark theme. Pinned tags show as a colour dot on every tile.
CREATE TABLE tags (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT        NOT NULL,
    color      TEXT        NOT NULL DEFAULT 'slate',
    pinned     BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX tags_name_key ON tags (lower(name));

CREATE TABLE file_tags (
    file_id BIGINT NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    tag_id  BIGINT NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    PRIMARY KEY (file_id, tag_id)
);
CREATE INDEX file_tags_tag_idx ON file_tags (tag_id);

-- Notes and provenance, one row per file that has any.
CREATE TABLE file_meta (
    file_id    BIGINT PRIMARY KEY REFERENCES files (id) ON DELETE CASCADE,
    notes      TEXT        NOT NULL DEFAULT '',
    source_url TEXT        NOT NULL DEFAULT '',
    designer   TEXT        NOT NULL DEFAULT '',
    license    TEXT        NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Collections nest. A collection with rules is smart: its files are whatever matches
-- the saved filter, so new files file themselves. Without rules it holds what was added.
CREATE TABLE collections (
    id         BIGSERIAL PRIMARY KEY,
    parent_id  BIGINT REFERENCES collections (id) ON DELETE SET NULL,
    name       TEXT        NOT NULL,
    rules      JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX collections_parent_idx ON collections (parent_id);
CREATE UNIQUE INDEX collections_sibling_name_key ON collections (coalesce(parent_id, 0), lower(name));

CREATE TABLE collection_files (
    collection_id BIGINT      NOT NULL REFERENCES collections (id) ON DELETE CASCADE,
    file_id       BIGINT      NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (collection_id, file_id)
);
CREATE INDEX collection_files_file_idx ON collection_files (file_id);

-- The print queue: named lists (optionally for one printer) of files to print.
CREATE TABLE print_lists (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT        NOT NULL,
    printer    TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX print_lists_name_key ON print_lists (lower(name));

CREATE TABLE print_items (
    id          BIGSERIAL PRIMARY KEY,
    list_id     BIGINT           NOT NULL REFERENCES print_lists (id) ON DELETE CASCADE,
    file_id     BIGINT           NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    quantity    INT              NOT NULL DEFAULT 1 CHECK (quantity BETWEEN 1 AND 999),
    status      TEXT             NOT NULL DEFAULT 'todo' CHECK (status IN ('todo', 'printing', 'done')),
    note        TEXT             NOT NULL DEFAULT '',
    position    DOUBLE PRECISION NOT NULL,
    added_at    TIMESTAMPTZ      NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    UNIQUE (list_id, file_id)
);
CREATE INDEX print_items_file_idx ON print_items (file_id);

-- Photos attached to a file. The cover one replaces the file's generated thumbnail.
CREATE TABLE file_photos (
    id           BIGSERIAL PRIMARY KEY,
    file_id      BIGINT      NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    path         TEXT        NOT NULL, -- the original, relative to DATA_DIR
    content_type TEXT        NOT NULL,
    thumb_path   TEXT        NOT NULL, -- a smaller copy for tiles
    thumb_type   TEXT        NOT NULL,
    width        INT         NOT NULL,
    height       INT         NOT NULL,
    size         BIGINT      NOT NULL,
    is_cover     BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX file_photos_file_idx ON file_photos (file_id);
CREATE UNIQUE INDEX file_photos_one_cover_key ON file_photos (file_id) WHERE is_cover;

-- A folder's cover is the thumbnail of a file chosen for it.
CREATE TABLE folder_covers (
    source_id BIGINT NOT NULL REFERENCES sources (id) ON DELETE CASCADE,
    dir       TEXT   NOT NULL,
    file_id   BIGINT NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    PRIMARY KEY (source_id, dir)
);
CREATE INDEX folder_covers_file_idx ON folder_covers (file_id);

CREATE INDEX files_missing_idx ON files (id) WHERE status = 'missing';

-- +goose Down
DROP INDEX files_missing_idx;
DROP TABLE folder_covers;
DROP TABLE file_photos;
DROP TABLE print_items;
DROP TABLE print_lists;
DROP TABLE collection_files;
DROP TABLE collections;
DROP TABLE file_meta;
DROP TABLE file_tags;
DROP TABLE tags;
