-- 消息任务与常用业务查询索引。请在低峰期执行。
-- 若 task_key 有重复，脚本会主动中止，不会静默删除业务数据。

DROP PROCEDURE IF EXISTS migrate_message_task_indexes;
DELIMITER $$
CREATE PROCEDURE migrate_message_task_indexes()
BEGIN
  IF EXISTS (
    SELECT task_key FROM email_task
    WHERE task_key IS NOT NULL AND task_key <> ''
    GROUP BY task_key HAVING COUNT(*) > 1 LIMIT 1
  ) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'duplicate email_task.task_key found; clean duplicates before migration';
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'email_task' AND index_name = 'uk_email_task_task_key'
  ) THEN
    ALTER TABLE email_task ADD UNIQUE INDEX uk_email_task_task_key (task_key);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'project_application' AND index_name = 'idx_pa_user_status'
  ) THEN
    ALTER TABLE project_application ADD INDEX idx_pa_user_status (user_id, status);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'olive_branch_record' AND index_name = 'idx_ob_receiver_status_created'
  ) THEN
    ALTER TABLE olive_branch_record ADD INDEX idx_ob_receiver_status_created (receiver_id, status, created_at);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'olive_branch_record' AND index_name = 'idx_ob_sender_updated'
  ) THEN
    ALTER TABLE olive_branch_record ADD INDEX idx_ob_sender_updated (sender_id, updated_at);
  END IF;
END$$
DELIMITER ;

CALL migrate_message_task_indexes();
DROP PROCEDURE IF EXISTS migrate_message_task_indexes;
