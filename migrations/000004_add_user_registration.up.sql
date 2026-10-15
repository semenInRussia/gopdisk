ALTER TABLE users
ADD COLUMN email TEXT,
ADD COLUMN password_hash BYTEA,
ADD COLUMN activated BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE users
SET email = 'legacy-' || id::text || '@invalid.local',
    password_hash = convert_to('!', 'UTF8')
WHERE email IS NULL;

ALTER TABLE users
ALTER COLUMN email SET NOT NULL,
ALTER COLUMN password_hash SET NOT NULL;

CREATE UNIQUE INDEX users_email_unique_idx ON users (lower(email));

CREATE TABLE tokens (
    hash BYTEA PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expiry TIMESTAMPTZ NOT NULL,
    scope TEXT NOT NULL
);

CREATE INDEX tokens_user_id_idx ON tokens(user_id);
