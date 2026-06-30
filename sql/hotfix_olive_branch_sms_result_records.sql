-- MySQL 5.7+ hotfix for olive branch invitation/rejected/accepted SMS records.
--
-- One olive branch can have an invitation SMS and a later result SMS. The old
-- unique index on olive_branch_record_id rejects the result record before the
-- message center is called, so email_task remains empty. Idempotency is scoped
-- to the paid order by uk_obsn_order instead.

DROP PROCEDURE IF EXISTS hotfix_olive_branch_sms_result_records;
DELIMITER $$
CREATE PROCEDURE hotfix_olive_branch_sms_result_records()
BEGIN
  DECLARE branch_unique_index VARCHAR(128) DEFAULT NULL;

  SELECT INDEX_NAME INTO branch_unique_index
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'olive_branch_sms_notice'
    AND COLUMN_NAME = 'olive_branch_record_id'
    AND NON_UNIQUE = 0
    AND INDEX_NAME <> 'PRIMARY'
  LIMIT 1;

  IF branch_unique_index IS NOT NULL THEN
    SET @drop_branch_unique = CONCAT(
      'ALTER TABLE olive_branch_sms_notice DROP INDEX `',
      REPLACE(branch_unique_index, '`', '``'),
      '`'
    );
    PREPARE stmt FROM @drop_branch_unique;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'olive_branch_sms_notice'
      AND INDEX_NAME = 'uk_obsn_order'
  ) THEN
    ALTER TABLE olive_branch_sms_notice
      ADD UNIQUE KEY uk_obsn_order (order_id);
  END IF;
END$$
DELIMITER ;

CALL hotfix_olive_branch_sms_result_records();
DROP PROCEDURE hotfix_olive_branch_sms_result_records;
