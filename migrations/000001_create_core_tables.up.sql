CREATE TABLE users (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE files (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    owner_id UUID NOT NULL REFERENCES users(id),
    filetype TEXT NOT NULL,
    size BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE blobs (
    hash TEXT PRIMARY KEY,
    size BIGINT NOT NULL
);

CREATE TABLE file_blobs (
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    blob_hash TEXT NOT NULL REFERENCES blobs(hash),
    position INT NOT NULL,

    PRIMARY KEY (file_id, position),
    UNIQUE (file_id, blob_hash, position),
    CHECK (position >= 0)
);

CREATE INDEX files_owner_id_idx ON files(owner_id);
CREATE INDEX file_blobs_blob_hash_idx ON file_blobs(blob_hash);

CREATE VIEW file_blob_view AS
SELECT
    f.id AS file_id,
    f.owner_id,
    f.name,
    f.filetype,
    f.size AS file_size,
    fb.position,
    b.hash AS blob_hash,
    b.size AS blob_size
FROM files f
JOIN file_blobs fb ON fb.file_id = f.id
JOIN blobs b ON b.hash = fb.blob_hash;
