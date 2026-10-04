ALTER TABLE blobs
DROP CONSTRAINT IF EXISTS blobs_size_non_negative;

ALTER TABLE files
DROP CONSTRAINT IF EXISTS files_size_non_negative;
