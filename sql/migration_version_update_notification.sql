-- 允许 last_viewed_at 为 NULL，标识从未查看。
CREATE TABLE IF NOT EXISTS `user_roadmap_view` (
  `user_id` INT UNSIGNED NOT NULL,
  `last_viewed_at` TIMESTAMP NULL COMMENT '最后查看时间，NULL代表从未查看',
  PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户进度看板已读时间';

-- 新增 roadmap 表 created_at 字段，幂等执行。
SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE `roadmap` ADD COLUMN `created_at` TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP',
    'SELECT 1'
  )
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'roadmap'
    AND COLUMN_NAME = 'created_at'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 新增 roadmap 表 updated_at 字段，幂等执行。
SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE `roadmap` ADD COLUMN `updated_at` TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP',
    'SELECT 1'
  )
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'roadmap'
    AND COLUMN_NAME = 'updated_at'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 新增消息模板表 page_path 跳转路径字段，幂等执行。
SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE `msg_template_config` ADD COLUMN `page_path` VARCHAR(255) DEFAULT NULL COMMENT ''点击订阅消息卡片后跳转的小程序页面路径''',
    'SELECT 1'
  )
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'msg_template_config'
    AND COLUMN_NAME = 'page_path'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 插入/更新版本更新通知消息模板，幂等执行。
INSERT INTO `msg_template_config` (`biz_key`, `template_id`, `template_title`, `content_json`, `page_path`)
VALUES (
  'MSG_VERSION_UPDATE',
  'E4chXELjSpL2SqcanY7ooXRIs365cyYad7I0hrbylmg',
  '版本更新通知',
  JSON_OBJECT('title', 'thing5', 'content', 'thing3', 'remark', 'thing6'),
  'pages/roadmap/roadmap'
)
ON DUPLICATE KEY UPDATE
  `template_id` = VALUES(`template_id`),
  `template_title` = VALUES(`template_title`),
  `content_json` = VALUES(`content_json`),
  `page_path` = VALUES(`page_path`),
  `updated_at` = CURRENT_TIMESTAMP;
