-- The matching UNIQUE indexes already cover these exact single-column lookups.
-- Safe to rerun and safe when one or more legacy indexes are already absent.

DROP PROCEDURE IF EXISTS _remove_redundant_indexes;
DELIMITER $$
CREATE PROCEDURE _remove_redundant_indexes()
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'admin_user'
      AND index_name = 'idx_admin_user_username'
  ) THEN
    DROP INDEX idx_admin_user_username ON admin_user;
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'talent_profile'
      AND index_name = 'idx_talent_user'
  ) THEN
    DROP INDEX idx_talent_user ON talent_profile;
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'user'
      AND index_name = 'idx_user_openid'
  ) THEN
    DROP INDEX idx_user_openid ON `user`;
  END IF;
END$$
DELIMITER ;

CALL _remove_redundant_indexes();
DROP PROCEDURE IF EXISTS _remove_redundant_indexes;
