-- Event detail upgrade (MySQL 8+, repeatable).
-- Run manually after review; this repository never executes migrations automatically.

DROP PROCEDURE IF EXISTS add_event_detail_column;
DELIMITER $$
CREATE PROCEDURE add_event_detail_column(IN column_name_value VARCHAR(64), IN column_definition_value TEXT)
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'event' AND COLUMN_NAME = column_name_value
  ) THEN
    SET @event_detail_ddl = CONCAT('ALTER TABLE `event` ADD COLUMN `', column_name_value, '` ', column_definition_value);
    PREPARE event_detail_stmt FROM @event_detail_ddl;
    EXECUTE event_detail_stmt;
    DEALLOCATE PREPARE event_detail_stmt;
  END IF;
END$$
DELIMITER ;

CALL add_event_detail_column('organizer_name', 'VARCHAR(200) NULL COMMENT ''主办方名称'' AFTER `summary`');
CALL add_event_detail_column('description', 'TEXT NULL COMMENT ''赛事简介'' AFTER `organizer_name`');
CALL add_event_detail_column('resource_url', 'VARCHAR(2048) NULL COMMENT ''资料池链接'' AFTER `description`');
CALL add_event_detail_column('qq_group', 'VARCHAR(32) NULL COMMENT ''QQ 群号'' AFTER `resource_url`');
CALL add_event_detail_column('allow_cross_school', 'TINYINT(1) NOT NULL DEFAULT 1 COMMENT ''是否允许跨校'' AFTER `qq_group`');
CALL add_event_detail_column('allow_cross_major', 'TINYINT(1) NOT NULL DEFAULT 1 COMMENT ''是否允许跨专业'' AFTER `allow_cross_school`');
CALL add_event_detail_column('view_count', 'BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT ''详情页 PV'' AFTER `allow_cross_major`');

DROP PROCEDURE IF EXISTS add_event_detail_column;

UPDATE `event` SET allow_cross_school = 1 WHERE allow_cross_school IS NULL;
UPDATE `event` SET allow_cross_major = 1 WHERE allow_cross_major IS NULL;

UPDATE `event` e
JOIN `school` s ON s.id = e.school_id
SET e.organizer_name = s.school_name
WHERE e.level = 'school' AND (e.organizer_name IS NULL OR e.organizer_name = '');

CREATE TABLE IF NOT EXISTS `event_timeline_node` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `event_id` INT UNSIGNED NOT NULL,
  `title` VARCHAR(120) NOT NULL,
  `node_time` DATETIME NOT NULL,
  `description` VARCHAR(500) NULL,
  `sort_order` INT NOT NULL DEFAULT 0,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_event_timeline_event_time` (`event_id`, `node_time`, `sort_order`, `id`),
  CONSTRAINT `fk_event_timeline_event` FOREIGN KEY (`event_id`) REFERENCES `event` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='赛事关键时间节点';

INSERT INTO `event_timeline_node` (`event_id`, `title`, `node_time`, `description`, `sort_order`)
SELECT e.id, '报名截止', TIMESTAMP(e.registration_deadline, '23:59:00'), '赛事报名截止时间', 0
FROM `event` e
WHERE e.registration_deadline IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM `event_timeline_node` n
    WHERE n.event_id = e.id AND n.title = '报名截止'
  );
