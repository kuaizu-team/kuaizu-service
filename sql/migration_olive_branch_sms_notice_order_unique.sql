-- Olive branch SMS notice order idempotency migration for MySQL 5.7+.
-- Safe to re-run after duplicate data has been cleaned.
-- If duplicate order_id records already exist, this script stops before ALTER TABLE.

DROP PROCEDURE IF EXISTS _olive_branch_sms_notice_order_unique;
DELIMITER $$
CREATE PROCEDURE _olive_branch_sms_notice_order_unique()
BEGIN
  IF EXISTS (
    SELECT 1
    FROM (
      SELECT order_id
      FROM olive_branch_sms_notice
      GROUP BY order_id
      HAVING COUNT(*) > 1
      LIMIT 1
    ) dup
  ) THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'olive_branch_sms_notice has duplicate order_id records; clean duplicates before adding uk_obsn_order';
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'olive_branch_sms_notice'
      AND index_name = 'uk_obsn_order'
  ) THEN
    ALTER TABLE olive_branch_sms_notice
      ADD UNIQUE KEY uk_obsn_order (order_id);
  END IF;
END$$
DELIMITER ;
CALL _olive_branch_sms_notice_order_unique();
DROP PROCEDURE IF EXISTS _olive_branch_sms_notice_order_unique;

