DROP VIEW IF EXISTS file_blob_view;

DROP INDEX IF EXISTS file_blobs_blob_hash_idx;
DROP INDEX IF EXISTS files_owner_id_idx;

DROP TABLE IF EXISTS file_blobs;
DROP TABLE IF EXISTS blobs;
DROP TABLE IF EXISTS files;
DROP TABLE IF EXISTS users;
