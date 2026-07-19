-- MySQL 5.7: speed up admin rating history and client project-score summaries.
DROP PROCEDURE IF EXISTS _add_admin_rating_history_index;
DELIMITER $$
CREATE PROCEDURE _add_admin_rating_history_index()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA=DATABASE()
      AND TABLE_NAME='project_member_rating'
      AND INDEX_NAME='idx_rating_target_history'
  ) THEN
    ALTER TABLE project_member_rating
      ADD KEY idx_rating_target_history (target_id, created_at, id);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA=DATABASE()
      AND TABLE_NAME='project_member_score'
      AND INDEX_NAME='idx_member_score_user_project'
  ) THEN
    ALTER TABLE project_member_score
      ADD KEY idx_member_score_user_project (member_id, project_id, updated_at);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA=DATABASE()
      AND TABLE_NAME='project_member_removal'
      AND INDEX_NAME='idx_member_removal_user_project_cycle'
  ) THEN
    ALTER TABLE project_member_removal
      ADD KEY idx_member_removal_user_project_cycle (user_id, project_id, joined_at, removed_at);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA=DATABASE()
      AND TABLE_NAME='collaboration_score'
      AND INDEX_NAME='idx_collaboration_score_user_project_created'
  ) THEN
    ALTER TABLE collaboration_score
      ADD KEY idx_collaboration_score_user_project_created (user_id, project_id, created_at, id);
  END IF;
END$$
DELIMITER ;
CALL _add_admin_rating_history_index();
DROP PROCEDURE IF EXISTS _add_admin_rating_history_index;