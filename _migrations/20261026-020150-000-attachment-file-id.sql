-- An attachment holds its managed file by ID. Attachments stored by path
-- before IDs keep storage_path until they are resolved when next read.
ALTER TABLE attachments ADD COLUMN file_id TEXT NOT NULL DEFAULT '';
