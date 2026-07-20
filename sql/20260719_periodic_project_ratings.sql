-- Periodic project member ratings (rolling 30-day cooldown), MySQL 5.7+.
-- Ratings are bound to project_members.id so leaving and rejoining starts a new,
-- independent rating cycle without deleting the audit trail from the old cycle.

INSERT INTO project_role (code, name, status, sort_order)
VALUES ('LEARNING_MEMBER', '学习成员', 1, 90)
ON DUPLICATE KEY UPDATE name=VALUES(name), status=VALUES(status), sort_order=VALUES(sort_order);

CREATE TABLE IF NOT EXISTS project_member_rating (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  project_id BIGINT UNSIGNED NOT NULL,
  rater_id BIGINT UNSIGNED NOT NULL COMMENT '评分用户 ID',
  target_id BIGINT UNSIGNED NOT NULL COMMENT '被评分用户 ID',
  rater_member_id BIGINT UNSIGNED NOT NULL COMMENT '评分人的本次成员关系 ID',
  target_member_id BIGINT UNSIGNED NOT NULL COMMENT '被评分人的本次成员关系 ID',
  rater_role VARCHAR(32) NOT NULL COMMENT '评分提交时的角色快照',
  rater_weight DECIMAL(3,2) NOT NULL COMMENT '评分提交时的角色权重快照',
  score TINYINT UNSIGNED NOT NULL COMMENT '0-100',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_rating_target_latest (target_member_id, rater_id, id),
  KEY idx_rating_cooldown (project_id, rater_id, target_member_id, created_at),
  KEY idx_rating_project_target (project_id, target_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='项目成员周期性互评明细';

CREATE TABLE IF NOT EXISTS project_member_score (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  project_id BIGINT UNSIGNED NOT NULL,
  project_member_id BIGINT UNSIGNED NOT NULL COMMENT '成员本次加入关系 ID',
  member_id BIGINT UNSIGNED NOT NULL COMMENT '成员用户 ID',
  score DECIMAL(5,2) DEFAULT NULL COMMENT '当前角色加权平均分，NULL 表示暂无评分',
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_project_member_score_cycle (project_member_id),
  KEY idx_project_member_score_lookup (project_id, member_id, project_member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='项目成员当前生效评分';

ALTER TABLE collaboration_score
  MODIFY COLUMN score DECIMAL(5,2) NOT NULL COMMENT '项目成员移除时固化的最终评分';

ALTER TABLE project_member_removal
  MODIFY COLUMN score DECIMAL(5,2) NULL COMMENT '项目成员移除时固化的最终评分';

DROP PROCEDURE IF EXISTS _periodic_rating_add_history_columns;
DELIMITER $$
CREATE PROCEDURE _periodic_rating_add_history_columns()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA=DATABASE()
      AND TABLE_NAME='collaboration_score'
      AND COLUMN_NAME='rating_count'
  ) THEN
    ALTER TABLE collaboration_score
      ADD COLUMN rating_count INT UNSIGNED NOT NULL DEFAULT 1
      COMMENT '固化评分包含的有效评分人数' AFTER score;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.STATISTICS
    WHERE TABLE_SCHEMA=DATABASE()
      AND TABLE_NAME='collaboration_score'
      AND INDEX_NAME='idx_collaboration_score_user_created'
  ) THEN
    ALTER TABLE collaboration_score
      ADD KEY idx_collaboration_score_user_created (user_id, created_at);
  END IF;
END$$
DELIMITER ;
CALL _periodic_rating_add_history_columns();
DROP PROCEDURE IF EXISTS _periodic_rating_add_history_columns;

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
