-- Project timeline and multi-role member support.
-- Compatible with MySQL 5.7.

INSERT INTO project_role (code, name, status, sort_order)
SELECT 'TEAM_MEMBER', '团队成员', 1, 80
FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM project_role WHERE code = 'TEAM_MEMBER');

CREATE TABLE IF NOT EXISTS project_milestones (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  project_id BIGINT UNSIGNED NOT NULL,
  milestone_date DATE NOT NULL,
  description VARCHAR(10) NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_project_milestones_project_date (project_id, milestone_date, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='项目里程碑时间线';

CREATE TABLE IF NOT EXISTS project_members (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  project_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  role VARCHAR(32) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_project_members_project_user (project_id, user_id),
  KEY idx_project_members_user (user_id),
  KEY idx_project_members_project_role (project_id, role)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='项目团队成员关系';

INSERT INTO project_members (project_id, user_id, role, created_at, updated_at)
SELECT p.id, p.creator_id, COALESCE(NULLIF(p.publisher_role, ''), 'TEAM_LEADER'), NOW(), NOW()
FROM project p
WHERE p.creator_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM project_members pm
    WHERE pm.project_id = p.id AND pm.user_id = p.creator_id
  );

SET @reviewer_id_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'project_application'
    AND COLUMN_NAME = 'reviewer_id'
);
SET @sql := IF(@reviewer_id_exists = 0,
  'ALTER TABLE project_application ADD COLUMN reviewer_id BIGINT UNSIGNED NULL AFTER is_read',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @reviewer_role_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'project_application'
    AND COLUMN_NAME = 'reviewer_role'
);
SET @sql := IF(@reviewer_role_exists = 0,
  'ALTER TABLE project_application ADD COLUMN reviewer_role VARCHAR(32) NULL AFTER reviewer_id',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @reviewer_idx_exists := (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'project_application'
    AND INDEX_NAME = 'idx_project_application_reviewer'
);
SET @sql := IF(@reviewer_idx_exists = 0,
  'ALTER TABLE project_application ADD KEY idx_project_application_reviewer (reviewer_id)',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @reviewer_role_idx_exists := (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'project_application'
    AND INDEX_NAME = 'idx_project_application_reviewer_role'
);
SET @sql := IF(@reviewer_role_idx_exists = 0,
  'ALTER TABLE project_application ADD KEY idx_project_application_reviewer_role (reviewer_role)',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
