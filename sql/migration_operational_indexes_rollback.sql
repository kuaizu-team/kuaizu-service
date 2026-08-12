-- Idempotent rollback for migration_operational_indexes.sql and
-- migration_remove_redundant_indexes.sql. Run only when reverting their query plan.

DROP PROCEDURE IF EXISTS _rollback_operational_indexes;
DELIMITER $$
CREATE PROCEDURE _rollback_operational_indexes()
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='project_view_log' AND index_name='idx_pvl_project_duration_viewed_user') THEN
    DROP INDEX idx_pvl_project_duration_viewed_user ON project_view_log;
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='project_view_log' AND index_name='idx_pvl_project_user_duration_viewed') THEN
    DROP INDEX idx_pvl_project_user_duration_viewed ON project_view_log;
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='talent_view_log' AND index_name='idx_tvl_talent_duration_viewed_user') THEN
    DROP INDEX idx_tvl_talent_duration_viewed_user ON talent_view_log;
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='talent_view_log' AND index_name='idx_tvl_talent_user_duration_viewed') THEN
    DROP INDEX idx_tvl_talent_user_duration_viewed ON talent_view_log;
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='email_task' AND index_name='idx_email_task_promotion_channel_status') THEN
    DROP INDEX idx_email_task_promotion_channel_status ON email_task;
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='email_task' AND index_name='idx_email_task_recipient_promotion') THEN
    DROP INDEX idx_email_task_recipient_promotion ON email_task;
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='email_promotion' AND index_name='idx_ep_reconcile') THEN
    DROP INDEX idx_ep_reconcile ON email_promotion;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='project_view_log' AND index_name='idx_pvl_project_viewed_at') THEN
    CREATE INDEX idx_pvl_project_viewed_at ON project_view_log (project_id, viewed_at);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='project_view_log' AND index_name='idx_pvl_project_user') THEN
    CREATE INDEX idx_pvl_project_user ON project_view_log (project_id, user_id);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='email_task' AND index_name='idx_promotion_id') THEN
    CREATE INDEX idx_promotion_id ON email_task (promotion_id);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='admin_user' AND index_name='idx_admin_user_username') THEN
    CREATE INDEX idx_admin_user_username ON admin_user (username);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='talent_profile' AND index_name='idx_talent_user') THEN
    CREATE INDEX idx_talent_user ON talent_profile (user_id);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='user' AND index_name='idx_user_openid') THEN
    CREATE INDEX idx_user_openid ON `user` (openid);
  END IF;
END$$
DELIMITER ;

CALL _rollback_operational_indexes();
DROP PROCEDURE IF EXISTS _rollback_operational_indexes;
