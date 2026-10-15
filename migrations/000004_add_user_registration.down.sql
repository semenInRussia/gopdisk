DROP INDEX IF EXISTS tokens_user_id_idx;
DROP TABLE IF EXISTS tokens;

DROP INDEX IF EXISTS users_email_unique_idx;

ALTER TABLE users
DROP COLUMN IF EXISTS activated,
DROP COLUMN IF EXISTS password_hash,
DROP COLUMN IF EXISTS email;
