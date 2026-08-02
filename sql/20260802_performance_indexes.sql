-- Performance indexes for profile badges, message reconciliation and admin dashboards.
-- Idempotent on MySQL 5.7/8.0. Execute during a low-traffic maintenance window.

DROP PROCEDURE IF EXISTS migrate_20260802_performance_indexes;
DELIMITER $$
CREATE PROCEDURE migrate_20260802_performance_indexes()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'email_task'
      AND index_name = 'idx_email_task_identity_latest'
  ) THEN
    ALTER TABLE email_task
      ADD INDEX idx_email_task_identity_latest (channel, business_tag, trace_id, id);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'project_application'
      AND index_name = 'idx_pa_user_updated_status'
  ) THEN
    ALTER TABLE project_application
      ADD INDEX idx_pa_user_updated_status (user_id, updated_at, status);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'project_application'
      AND index_name = 'idx_pa_status_project'
  ) THEN
    ALTER TABLE project_application
      ADD INDEX idx_pa_status_project (status, project_id);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'project'
      AND index_name = 'idx_project_school_status'
  ) THEN
    ALTER TABLE project
      ADD INDEX idx_project_school_status (school_id, status, id);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'user'
      AND index_name = 'idx_user_school_auth'
  ) THEN
    ALTER TABLE `user`
      ADD INDEX idx_user_school_auth (school_id, auth_status, id);
  END IF;
END$$
DELIMITER ;

CALL migrate_20260802_performance_indexes();
DROP PROCEDURE IF EXISTS migrate_20260802_performance_indexes;
