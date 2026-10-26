-- MySQL 8.0.13+. Manual migration, NOT executed by Codex.
-- Select the intended database and back up campus_cooperation before running.
-- Add only cooperation tags and deduplicated readership; no application/link/contact changes.
-- Execute BEFORE deploying the matching backend. Existing readers start at 0; no fabricated history.
-- Safe to repeat. Existing tags and view rows are preserved.

DROP PROCEDURE IF EXISTS migrate_cooperation_cards_20261026;
DELIMITER $$
CREATE PROCEDURE migrate_cooperation_cards_20261026()
BEGIN
  IF DATABASE() IS NULL THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Select the intended database first';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='project_tag') THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Existing project_tag library is required';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.COLUMNS
      WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='campus_cooperation' AND COLUMN_NAME='tags') THEN
    ALTER TABLE campus_cooperation ADD COLUMN tags JSON NULL
      COMMENT 'Ordered tag names shared with project_tag; 0-5 tags, each 1-12 characters';
  END IF;
END$$
DELIMITER ;
CALL migrate_cooperation_cards_20261026();
DROP PROCEDURE migrate_cooperation_cards_20261026;

CREATE TABLE IF NOT EXISTS campus_cooperation_view (
  cooperation_id BIGINT NOT NULL,
  user_id INT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (cooperation_id,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='Unique authenticated school-authorized readers; repeated detail reads do not increase readership';

-- Read-only verification. The primary key must contain cooperation_id,user_id.
SHOW COLUMNS FROM campus_cooperation LIKE 'tags';
SHOW CREATE TABLE campus_cooperation_view;
SELECT c.id,c.title,c.tags,
       (SELECT COUNT(*) FROM campus_cooperation_view v WHERE v.cooperation_id=c.id) AS view_count
FROM campus_cooperation c ORDER BY c.id;
