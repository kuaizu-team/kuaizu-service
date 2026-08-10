-- Suggested operational indexes for dashboard, reconciliation and recipient queries.
-- Review and execute manually during a maintenance window; this file is not run automatically.

DROP PROCEDURE IF EXISTS _optimize_operational_indexes;
DELIMITER $$
CREATE PROCEDURE _optimize_operational_indexes()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'project_view_log'
      AND index_name = 'idx_pvl_project_duration_viewed_user'
  ) THEN
    CREATE INDEX idx_pvl_project_duration_viewed_user
      ON project_view_log (project_id, duration_ms, viewed_at, user_id, id);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'project_view_log'
      AND index_name = 'idx_pvl_project_user_duration_viewed'
  ) THEN
    CREATE INDEX idx_pvl_project_user_duration_viewed
      ON project_view_log (project_id, user_id, duration_ms, viewed_at, id);
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'project_view_log'
      AND index_name = 'idx_pvl_project_viewed_at'
  ) THEN
    DROP INDEX idx_pvl_project_viewed_at ON project_view_log;
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'project_view_log'
      AND index_name = 'idx_pvl_project_user'
  ) THEN
    DROP INDEX idx_pvl_project_user ON project_view_log;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'talent_view_log'
      AND index_name = 'idx_tvl_talent_duration_viewed_user'
  ) THEN
    CREATE INDEX idx_tvl_talent_duration_viewed_user
      ON talent_view_log (talent_id, duration_ms, viewed_at, user_id, id);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'talent_view_log'
      AND index_name = 'idx_tvl_talent_user_duration_viewed'
  ) THEN
    CREATE INDEX idx_tvl_talent_user_duration_viewed
      ON talent_view_log (talent_id, user_id, duration_ms, viewed_at, id);
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'talent_view_log'
      AND index_name = 'idx_talent_viewed_at'
  ) THEN
    DROP INDEX idx_talent_viewed_at ON talent_view_log;
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'talent_view_log'
      AND index_name = 'idx_talent_user'
  ) THEN
    DROP INDEX idx_talent_user ON talent_view_log;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'email_task'
      AND index_name = 'idx_email_task_promotion_channel_status'
  ) THEN
    CREATE INDEX idx_email_task_promotion_channel_status
      ON email_task (promotion_id, channel, status);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'email_task'
      AND index_name = 'idx_email_task_recipient_promotion'
  ) THEN
    CREATE INDEX idx_email_task_recipient_promotion
      ON email_task (recipient_email, promotion_id);
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'email_task'
      AND index_name = 'idx_promotion_id'
  ) THEN
    DROP INDEX idx_promotion_id ON email_task;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'email_promotion'
      AND index_name = 'idx_ep_reconcile'
  ) THEN
    CREATE INDEX idx_ep_reconcile
      ON email_promotion (channel, business_tag, status, id);
  END IF;
END$$
DELIMITER ;

CALL _optimize_operational_indexes();
DROP PROCEDURE IF EXISTS _optimize_operational_indexes;
