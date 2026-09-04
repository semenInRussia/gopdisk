ALTER TABLE files
ADD COLUMN status TEXT NOT NULL DEFAULT 'ready';

ALTER TABLE files
ADD CONSTRAINT files_status_check
CHECK (status IN ('uploading', 'ready'));
