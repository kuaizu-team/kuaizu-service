-- Emergency schema compatibility for temporarily starting the previous binary.
-- This does NOT restore reversible ciphertext. All existing password_hash values
-- remain authoritative. Do not populate this column from a backup. While a
-- reversible-password binary is running, administrator creation/password reset
-- and event-manager credential creation/editing must be technically blocked as
-- required by docs/production-deploy.md; do not rely on operator convention.

DROP PROCEDURE IF EXISTS _restore_admin_password_compat_column;
DELIMITER $$
CREATE PROCEDURE _restore_admin_password_compat_column()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'admin_user'
      AND column_name = 'password_encrypted'
  ) THEN
    ALTER TABLE admin_user
      ADD COLUMN password_encrypted VARCHAR(500) NULL AFTER password_hash;
  END IF;
END$$
DELIMITER ;

CALL _restore_admin_password_compat_column();
DROP PROCEDURE IF EXISTS _restore_admin_password_compat_column;
