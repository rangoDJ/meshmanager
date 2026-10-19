-- +goose Up
-- NULL when not checked (very large meshes, or only the slicer preview was used).
ALTER TABLE model_info ADD COLUMN watertight BOOLEAN;

-- Files inside archives are indexed like folders: rel_path is "<archive rel_path>/<member>".
ALTER TABLE files
    ADD COLUMN archive_id      BIGINT REFERENCES files (id) ON DELETE CASCADE,
    ADD COLUMN member_path     TEXT,
    ADD COLUMN members_indexed BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX files_archive_id_idx ON files (archive_id) WHERE archive_id IS NOT NULL;

-- +goose Down
DROP INDEX files_archive_id_idx;
ALTER TABLE files DROP COLUMN members_indexed, DROP COLUMN member_path, DROP COLUMN archive_id;
ALTER TABLE model_info DROP COLUMN watertight;
