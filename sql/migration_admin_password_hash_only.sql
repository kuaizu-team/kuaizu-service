-- Deploy the hash-only application code before applying this migration.
-- This migration is intentionally not rerunnable.
UPDATE admin_user
SET password_encrypted = NULL
WHERE password_encrypted IS NOT NULL;

ALTER TABLE admin_user
DROP COLUMN password_encrypted;
