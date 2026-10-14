-- Reviewed against preflight receipt: MySQL 8.0.13, 2026-10-14. Not executed.
-- Review manually. This file has NOT been executed by the agent.
-- Run in the intended database BEFORE deploying cooperation backend handlers.
-- No existing rows, priorities, read flags, subscription configuration or payments are changed.
-- Four new tables; one nullable queue reference. Safe to repeat after a partial DDL run.
-- Actual existing column definitions must be verified with the companion verify script.

DROP PROCEDURE IF EXISTS preflight_campus_cooperation_migration;
DELIMITER $$
CREATE PROCEDURE preflight_campus_cooperation_migration()
BEGIN
  IF DATABASE() IS NULL THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Select the intended database first';
  END IF;
  IF (SELECT COUNT(*) FROM information_schema.TABLES
      WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN
      ('user','school','admin_user','admin_school_relation','status_notification','user_status_message')) <> 6 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Required existing business/message tables are missing';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.COLUMNS
      WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='status_notification'
      AND COLUMN_NAME='application_id' AND IS_NULLABLE='YES') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Apply the existing nullable status_notification application migration first';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.COLUMNS
      WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='admin_school_relation' AND COLUMN_NAME='is_owner') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Required authoritative school-owner column is missing';
  END IF;
END$$
DELIMITER ;
CALL preflight_campus_cooperation_migration();
DROP PROCEDURE preflight_campus_cooperation_migration;

CREATE TABLE IF NOT EXISTS commercial_order (
  id BIGINT NOT NULL AUTO_INCREMENT,
  name VARCHAR(100) NOT NULL,
  company_name VARCHAR(100) NOT NULL,
  description TEXT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'available' COMMENT 'available or closed; controls new school claims only',
  created_by INT NULL COMMENT 'admin_user audit identity, not authorization',
  updated_by INT NULL COMMENT 'admin_user audit identity',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_commercial_order_pool (status,created_at,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Platform commercial learning opportunities; unrelated to payment orders';

CREATE TABLE IF NOT EXISTS campus_cooperation (
  id BIGINT NOT NULL AUTO_INCREMENT,
  commercial_order_id BIGINT NOT NULL,
  school_id INT NOT NULL,
  claimed_by INT NULL COMMENT 'Original admin claimant; current owner controls later editing',
  claimed_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  title VARCHAR(20) NULL COMMENT 'Required when saving publishable content; at most 20 characters',
  introduction TEXT NULL COMMENT 'Required when saving publishable content',
  links_visible TINYINT(1) NOT NULL DEFAULT 0,
  links JSON NULL COMMENT 'Ordered array of {title,url}; visible links require valid URLs',
  images_visible TINYINT(1) NOT NULL DEFAULT 0,
  image_title VARCHAR(100) NULL,
  images JSON NULL COMMENT 'Ordered array of object keys belonging to this cooperation',
  contacts_visible TINYINT(1) NOT NULL DEFAULT 1,
  contacts JSON NULL COMMENT 'Ordered array of {type,label,value}; NEVER returned before approval in application mode',
  application_required TINYINT(1) NOT NULL DEFAULT 0,
  published TINYINT(1) NOT NULL DEFAULT 0 COMMENT 'New school claim starts as draft',
  updated_by INT NULL COMMENT 'admin_user audit identity',
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_campus_cooperation_school (commercial_order_id,school_id),
  KEY idx_campus_cooperation_school (school_id,published,claimed_at,id),
  CONSTRAINT fk_campus_cooperation_order FOREIGN KEY (commercial_order_id) REFERENCES commercial_order(id) ON DELETE RESTRICT,
  CONSTRAINT fk_campus_cooperation_school FOREIGN KEY (school_id) REFERENCES school(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Independent school claims and school-customized cooperation content';

CREATE TABLE IF NOT EXISTS campus_cooperation_application (
  id BIGINT NOT NULL AUTO_INCREMENT,
  cooperation_id BIGINT NOT NULL,
  user_id INT NOT NULL,
  attempt_no INT NOT NULL DEFAULT 1 COMMENT 'Reapplication after rejection creates a NEW row',
  status VARCHAR(20) NOT NULL DEFAULT 'pending' COMMENT 'pending, approved, rejected',
-- VIRTUAL preserves the unique active-application index while permitting user_id ON DELETE CASCADE.
-- MySQL disallows cascading foreign keys on base columns of STORED generated columns.
  active_user_id INT GENERATED ALWAYS AS
    (CASE WHEN status IN ('pending','approved') THEN user_id ELSE NULL END) VIRTUAL,
  profile_snapshot JSON NULL COMMENT 'Server-generated card snapshot; only applicant and authorized reviewers may read',
  applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  reviewed_by INT NULL COMMENT 'admin_user audit identity',
  reviewed_at DATETIME NULL DEFAULT NULL,
  review_remark VARCHAR(500) NULL COMMENT 'Optional internal review note; no applicant reason field',
  PRIMARY KEY (id),
  UNIQUE KEY uk_campus_application_attempt (cooperation_id,user_id,attempt_no),
  UNIQUE KEY uk_campus_application_active (cooperation_id,active_user_id),
  KEY idx_campus_application_user (user_id,applied_at,id),
  KEY idx_campus_application_review (cooperation_id,status,applied_at,id),
  CONSTRAINT fk_campus_application_cooperation FOREIGN KEY (cooperation_id) REFERENCES campus_cooperation(id) ON DELETE RESTRICT,
  CONSTRAINT fk_campus_application_user FOREIGN KEY (user_id) REFERENCES `user`(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='All cooperation application attempts, including prior rejected attempts';

CREATE TABLE IF NOT EXISTS campus_cooperation_image (
  object_key VARCHAR(255) NOT NULL,
  cooperation_id BIGINT NOT NULL,
  uploaded_by INT NULL COMMENT 'admin_user identity; do not use ordinary-user media_upload ownership',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (object_key),
  KEY idx_campus_cooperation_image (cooperation_id,created_at),
  CONSTRAINT fk_campus_image_cooperation FOREIGN KEY (cooperation_id) REFERENCES campus_cooperation(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='School cooperation image upload ownership';

DROP PROCEDURE IF EXISTS migrate_campus_cooperation_queue;
DELIMITER $$
CREATE PROCEDURE migrate_campus_cooperation_queue()
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.COLUMNS
      WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='status_notification' AND COLUMN_NAME='cooperation_application_id') THEN
    ALTER TABLE status_notification ADD COLUMN cooperation_application_id BIGINT NULL
      COMMENT 'Cooperation application source; existing notifications keep NULL';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.STATISTICS
      WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='status_notification' AND INDEX_NAME='idx_status_notification_cooperation') THEN
    ALTER TABLE status_notification ADD KEY idx_status_notification_cooperation (cooperation_application_id);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.TABLE_CONSTRAINTS
      WHERE CONSTRAINT_SCHEMA=DATABASE() AND TABLE_NAME='status_notification' AND CONSTRAINT_NAME='fk_status_notification_cooperation') THEN
    ALTER TABLE status_notification ADD CONSTRAINT fk_status_notification_cooperation
      FOREIGN KEY (cooperation_application_id) REFERENCES campus_cooperation_application(id) ON DELETE SET NULL;
  END IF;
END$$
DELIMITER ;
CALL migrate_campus_cooperation_queue();
DROP PROCEDURE migrate_campus_cooperation_queue;

-- Template initialization is separate: 20261014_02_campus_cooperation_template.sql (disabled).
-- Intentionally NO priority UPDATE and NO historical backfill: this is a new business line.
