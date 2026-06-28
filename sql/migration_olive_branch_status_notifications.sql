-- MySQL 5.7. Run after migration_status_notification.sql.
DROP PROCEDURE IF EXISTS migrate_olive_branch_status_notifications;
DELIMITER $$
CREATE PROCEDURE migrate_olive_branch_status_notifications()
BEGIN
	DECLARE branch_unique_index VARCHAR(128) DEFAULT NULL;
  IF NOT EXISTS (SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='olive_branch_record' AND COLUMN_NAME='discussing_at') THEN
    ALTER TABLE olive_branch_record ADD COLUMN discussing_at TIMESTAMP NULL DEFAULT NULL COMMENT 'entered discussing time' AFTER status;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='olive_branch_record' AND COLUMN_NAME='rejected_at') THEN
    ALTER TABLE olive_branch_record ADD COLUMN rejected_at TIMESTAMP NULL DEFAULT NULL COMMENT 'rejected time' AFTER discussing_at;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='olive_branch_record' AND COLUMN_NAME='admitted_at') THEN
    ALTER TABLE olive_branch_record ADD COLUMN admitted_at TIMESTAMP NULL DEFAULT NULL COMMENT 'admitted time' AFTER rejected_at;
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='status_notification' AND COLUMN_NAME='application_id' AND IS_NULLABLE='NO') THEN
    ALTER TABLE status_notification MODIFY application_id INT NULL;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='status_notification' AND COLUMN_NAME='olive_branch_id') THEN
    ALTER TABLE status_notification ADD COLUMN olive_branch_id INT NULL AFTER application_id;
    ALTER TABLE status_notification ADD KEY idx_status_notification_olive (olive_branch_id);
    ALTER TABLE status_notification ADD CONSTRAINT fk_status_notification_olive FOREIGN KEY (olive_branch_id) REFERENCES olive_branch_record(id) ON DELETE CASCADE;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='status_notification' AND COLUMN_NAME='priority') THEN
    ALTER TABLE status_notification ADD COLUMN priority INT NOT NULL DEFAULT 10 AFTER olive_branch_id;
  END IF;
	SELECT INDEX_NAME INTO branch_unique_index
	FROM information_schema.STATISTICS
	WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='olive_branch_sms_notice'
	  AND COLUMN_NAME='olive_branch_record_id' AND NON_UNIQUE=0 AND INDEX_NAME<>'PRIMARY'
	LIMIT 1;
	IF branch_unique_index IS NOT NULL THEN
		SET @drop_branch_unique = CONCAT('ALTER TABLE olive_branch_sms_notice DROP INDEX `', REPLACE(branch_unique_index, '`', '``'), '`');
		PREPARE stmt FROM @drop_branch_unique;
		EXECUTE stmt;
		DEALLOCATE PREPARE stmt;
	END IF;
END$$
DELIMITER ;
CALL migrate_olive_branch_status_notifications();
DROP PROCEDURE migrate_olive_branch_status_notifications;

UPDATE status_notification SET priority=100 WHERE type IN ('application-accepted','olive-accepted');
