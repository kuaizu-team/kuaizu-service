-- Forward-only security migration. Deploy and verify the hash-only application
-- before running this file. The removed ciphertext must never be restored.
-- The preflight in migration_admin_password_hash_only_verify.sql must PASS.
-- Dropping the column directly avoids a recoverability gap between UPDATE and DDL.

DROP PROCEDURE IF EXISTS _drop_admin_password_encrypted;
DELIMITER $$
CREATE PROCEDURE _drop_admin_password_encrypted()
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'admin_user'
      AND column_name = 'password_encrypted'
  ) THEN
    ALTER TABLE admin_user DROP COLUMN password_encrypted;
  END IF;
END$$
DELIMITER ;

CALL _drop_admin_password_encrypted();
DROP PROCEDURE IF EXISTS _drop_admin_password_encrypted;
