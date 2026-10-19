-- +goose Up
-- Where the source folder lives on the host or network (e.g. /mnt/nas/prints or
-- \\nas\prints), so the UI can show and copy a file's real location.
ALTER TABLE sources ADD COLUMN host_path TEXT;

-- +goose Down
ALTER TABLE sources DROP COLUMN host_path;
