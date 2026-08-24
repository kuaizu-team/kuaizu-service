-- Project image galleries, talent work images, and milestone certification.
-- Compatible with MySQL 5.7 and safe to run repeatedly.
-- This migration intentionally does not execute any data deletion.

SET @milestone_title_exists := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'project_milestones' AND COLUMN_NAME = 'title'
);
SET @sql := IF(@milestone_title_exists = 0,
  'ALTER TABLE project_milestones ADD COLUMN title VARCHAR(10) NULL AFTER milestone_date',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Existing description values are the historical 10-character node labels.
UPDATE project_milestones
SET title = LEFT(description, 10)
WHERE title IS NULL OR title = '';

SET @milestone_detail_exists := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'project_milestones' AND COLUMN_NAME = 'detail_description'
);
SET @sql := IF(@milestone_detail_exists = 0,
  'ALTER TABLE project_milestones ADD COLUMN detail_description VARCHAR(40) NOT NULL DEFAULT '''' AFTER description',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @milestone_cert_status_exists := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'project_milestones' AND COLUMN_NAME = 'certification_status'
);
SET @sql := IF(@milestone_cert_status_exists = 0,
  'ALTER TABLE project_milestones ADD COLUMN certification_status TINYINT NOT NULL DEFAULT 0 COMMENT ''0未认证 1待审核 2已认证 3已驳回'' AFTER detail_description',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

CREATE TABLE IF NOT EXISTS project_milestone_evidence (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  milestone_id BIGINT UNSIGNED NOT NULL,
  object_key VARCHAR(512) NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_milestone_evidence_key (object_key),
  KEY idx_milestone_evidence_milestone (milestone_id, sort_order, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='时间节点认证临时佐证图片';

CREATE TABLE IF NOT EXISTS media_upload (
  object_key VARCHAR(512) NOT NULL,
  owner_user_id BIGINT UNSIGNED NOT NULL,
  media_type VARCHAR(32) NOT NULL,
  attached_type VARCHAR(32) NULL,
  attached_id BIGINT UNSIGNED NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  attached_at DATETIME NULL,
  PRIMARY KEY (object_key),
  KEY idx_media_upload_owner_type (owner_user_id, media_type, created_at),
  KEY idx_media_upload_attachment (attached_type, attached_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='业务图片上传归属与关联登记';

CREATE TABLE IF NOT EXISTS project_image (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  project_id BIGINT UNSIGNED NOT NULL,
  object_key VARCHAR(512) NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_project_image_key (object_key),
  KEY idx_project_image_project (project_id, sort_order, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='项目图片集';

CREATE TABLE IF NOT EXISTS talent_work_image (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  talent_profile_id BIGINT UNSIGNED NOT NULL,
  object_key VARCHAR(512) NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_talent_work_image_key (object_key),
  KEY idx_talent_work_image_profile (talent_profile_id, sort_order, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='人才名片作品图片';

SET @milestone_cert_idx_exists := (
  SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'project_milestones'
    AND INDEX_NAME = 'idx_project_milestones_cert_status'
);
SET @sql := IF(@milestone_cert_idx_exists = 0,
  'ALTER TABLE project_milestones ADD KEY idx_project_milestones_cert_status (certification_status, updated_at, id)',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
