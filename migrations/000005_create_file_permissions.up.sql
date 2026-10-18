CREATE TABLE file_permissions (
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code TEXT NOT NULL CHECK (code IN ('files:read', 'files:write', 'files:delete')),
    PRIMARY KEY (file_id, user_id, code)
);

CREATE INDEX file_permissions_user_id_idx ON file_permissions(user_id);
