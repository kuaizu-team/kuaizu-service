-- Project application flow upgrade: pending -> discussing -> joined/rejected.
-- Compatible with MySQL 5.7; every ALTER is guarded through information_schema.

CREATE TABLE IF NOT EXISTS project_role (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  code VARCHAR(32) NOT NULL,
  name VARCHAR(32) NOT NULL,
  status TINYINT NOT NULL DEFAULT 1,
  sort_order INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_project_role_code (code),
  KEY idx_project_role_status_sort (status, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='项目团队角色字典';

INSERT INTO project_role (code, name, status, sort_order)
VALUES
  ('TEAM_LEADER', '团队负责人', 1, 10),
  ('TECH_LEADER', '技术负责人', 1, 20),
  ('OPERATIONS_LEADER', '运营负责人', 1, 30),
  ('PUBLICITY_LEADER', '宣传负责人', 1, 40),
  ('RECRUITMENT_LEADER', '招募负责人', 1, 50),
  ('DESIGN_LEADER', '美化负责人', 1, 60),
  ('LEGAL_LEADER', '法务负责人', 1, 70),
  ('TEAM_MEMBER', '团队成员', 1, 80)
ON DUPLICATE KEY UPDATE name = VALUES(name), status = VALUES(status), sort_order = VALUES(sort_order);

SET @publisher_role_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'project'
    AND COLUMN_NAME = 'publisher_role'
);
SET @sql := IF(@publisher_role_exists = 0,
  'ALTER TABLE project ADD COLUMN publisher_role VARCHAR(32) NULL AFTER skill_requirement',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @initiating_school_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'project'
    AND COLUMN_NAME = 'initiating_school_id'
);
SET @sql := IF(@initiating_school_exists = 0,
  'ALTER TABLE project ADD COLUMN initiating_school_id INT NULL AFTER publisher_role',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
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

SET @is_read_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'project_application'
    AND COLUMN_NAME = 'is_read'
);
SET @sql := IF(@is_read_exists = 0,
  'ALTER TABLE project_application ADD COLUMN is_read TINYINT(1) NOT NULL DEFAULT 0 AFTER reply_msg',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

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

SET @assigned_role_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'project_application'
    AND COLUMN_NAME = 'assigned_role'
);
SET @sql := IF(@assigned_role_exists = 0,
  'ALTER TABLE project_application ADD COLUMN assigned_role VARCHAR(32) NULL AFTER reviewer_role',
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

SET @assigned_role_idx_exists := (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'project_application'
    AND INDEX_NAME = 'idx_project_application_assigned_role'
);
SET @sql := IF(@assigned_role_idx_exists = 0,
  'ALTER TABLE project_application ADD KEY idx_project_application_assigned_role (assigned_role)',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

UPDATE project_application
SET assigned_role = NULL
WHERE status <> 3 AND assigned_role IS NOT NULL;
