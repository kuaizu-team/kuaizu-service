-- MySQL 5.7+. Review and execute manually before deploying the new API.
-- No foreign key to project_members: leaving/rejoining must retain history.
CREATE TABLE IF NOT EXISTS project_member_timeline (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  project_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  title VARCHAR(60) NOT NULL,
  detail TEXT NOT NULL,
  related_members JSON NOT NULL COMMENT 'Array of userId/nickname snapshots',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_member_timeline_project_user_time (project_id, user_id, created_at, id),
  KEY idx_member_timeline_project_time (project_id, created_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='成员个人进度时间线，按项目和用户保留';
