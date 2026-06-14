CREATE TABLE IF NOT EXISTS `roadmap` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `date` date NOT NULL COMMENT '发布日期',
  `title` varchar(100) NOT NULL COMMENT '标题',
  `content` text NOT NULL COMMENT '详细内容',
  `link` varchar(500) DEFAULT NULL COMMENT '公众号链接',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_roadmap_date` (`date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='平台进度看板';

SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE `feedback` ADD COLUMN `need_receipt` tinyint(1) NOT NULL DEFAULT ''0'' COMMENT ''是否接收回执/允许联系'' AFTER `email`',
    'SELECT 1'
  )
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'feedback'
    AND COLUMN_NAME = 'need_receipt'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
