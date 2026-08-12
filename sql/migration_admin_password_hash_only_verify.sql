-- Read-only preflight/postflight for the hash-only administrator migration.
-- Before migration: invalid_hash_count=0 is mandatory.
-- After migration: verification_status must be PASS.

SELECT
  COUNT(*) AS total_admins,
  SUM(password_hash IS NULL OR password_hash = ''
      OR password_hash NOT REGEXP '^\\$2[aby]\\$[0-9]{2}\\$.{53}$') AS invalid_hash_count
FROM admin_user;

SELECT
  CASE WHEN EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'admin_user'
      AND column_name = 'password_encrypted'
  ) THEN 'PRE_MIGRATION'
  ELSE 'PASS'
  END AS verification_status;
