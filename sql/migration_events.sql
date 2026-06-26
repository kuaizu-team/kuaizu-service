-- Campus event feature migration (MySQL 5.7 compatible)
-- Adds event library, project-event relation, and information-event relation.

CREATE TABLE IF NOT EXISTS `event` (
  `id` INT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name` VARCHAR(200) NOT NULL COMMENT '赛事全称',
  `is_ranking` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否为榜单赛事',
  `registration_deadline` DATE NULL COMMENT '报名截止日期',
  `article_url` VARCHAR(500) NULL COMMENT '关联公众号文章链接',
  `display_order` INT NOT NULL DEFAULT 0 COMMENT '展示权重（越高越靠右）',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_event_order` (`display_order`, `created_at`),
  KEY `idx_event_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='校园赛事库';

CREATE TABLE IF NOT EXISTS `project_event` (
  `id` INT UNSIGNED NOT NULL AUTO_INCREMENT,
  `project_id` INT UNSIGNED NOT NULL,
  `event_id` INT UNSIGNED NOT NULL,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_project_event` (`project_id`, `event_id`),
  KEY `idx_project_event_event` (`event_id`, `project_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='项目赛事关联';

CREATE TABLE IF NOT EXISTS `information_event` (
  `id` INT UNSIGNED NOT NULL AUTO_INCREMENT,
  `information_id` INT UNSIGNED NOT NULL,
  `event_id` INT UNSIGNED NOT NULL,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_information_event` (`information_id`, `event_id`),
  KEY `idx_information_event_event` (`event_id`, `information_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='资讯赛事关联';

-- Optional compatibility backfill: turn existing campus_event information rows into event rows.
INSERT INTO `event` (`name`, `is_ranking`, `article_url`, `display_order`, `created_at`, `updated_at`)
SELECT i.`title`, 0, NULLIF(i.`url`, ''), i.`display_order`, i.`created_at`, i.`updated_at`
FROM `information_content` i
WHERE i.`category` = 'campus_event'
  AND NOT EXISTS (
    SELECT 1 FROM `event` e WHERE e.`name` = i.`title`
  );

INSERT IGNORE INTO `information_event` (`information_id`, `event_id`)
SELECT i.`id`, e.`id`
FROM `information_content` i
JOIN `event` e ON e.`name` = i.`title`
WHERE i.`category` = 'campus_event';
