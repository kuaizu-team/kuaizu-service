-- MySQL 5.7. Run after migration_olive_branch_status_notifications.sql.
CREATE TABLE IF NOT EXISTS project_member_removal (
  id BIGINT NOT NULL AUTO_INCREMENT,
  user_id INT NOT NULL,
  project_id INT NOT NULL,
  operator_id INT NOT NULL,
  role VARCHAR(32) NOT NULL,
  joined_at DATETIME NOT NULL,
  removed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  score INT NULL,
  PRIMARY KEY (id),
  KEY idx_member_removal_user (user_id, removed_at),
  KEY idx_member_removal_project (project_id),
  CONSTRAINT fk_member_removal_user FOREIGN KEY (user_id) REFERENCES `user`(id) ON DELETE CASCADE,
  CONSTRAINT fk_member_removal_project FOREIGN KEY (project_id) REFERENCES project(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Removed project member snapshots';

DROP PROCEDURE IF EXISTS migrate_member_removal_notification;
DELIMITER $$
CREATE PROCEDURE migrate_member_removal_notification()
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='status_notification' AND COLUMN_NAME='member_removal_id') THEN
    ALTER TABLE status_notification ADD COLUMN member_removal_id BIGINT NULL AFTER olive_branch_id;
    ALTER TABLE status_notification ADD KEY idx_status_notification_member_removal (member_removal_id);
    ALTER TABLE status_notification ADD CONSTRAINT fk_status_notification_member_removal FOREIGN KEY (member_removal_id) REFERENCES project_member_removal(id) ON DELETE CASCADE;
  END IF;
	IF EXISTS (SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='olive_branch_sms_notice' AND COLUMN_NAME='olive_branch_record_id' AND IS_NULLABLE='NO') THEN
		ALTER TABLE olive_branch_sms_notice MODIFY olive_branch_record_id INT NULL;
	END IF;
	IF NOT EXISTS (SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='olive_branch_sms_notice' AND COLUMN_NAME='member_removal_id') THEN
		ALTER TABLE olive_branch_sms_notice ADD COLUMN member_removal_id BIGINT NULL AFTER olive_branch_record_id;
		ALTER TABLE olive_branch_sms_notice ADD KEY idx_sms_member_removal (member_removal_id);
		ALTER TABLE olive_branch_sms_notice ADD CONSTRAINT fk_sms_member_removal FOREIGN KEY (member_removal_id) REFERENCES project_member_removal(id) ON DELETE SET NULL;
	END IF;
END$$
DELIMITER ;
CALL migrate_member_removal_notification();
DROP PROCEDURE migrate_member_removal_notification;
