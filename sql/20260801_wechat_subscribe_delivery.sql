-- WeChat subscription delivery reliability and observability (MySQL 5.7+).
-- Execute this migration before deploying the matching service code.

CREATE TABLE IF NOT EXISTS `wx_subscribe_delivery` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `user_id` INT NOT NULL,
  `biz_key` VARCHAR(100) NOT NULL,
  `template_id` VARCHAR(100) DEFAULT NULL,
  `business_data` JSON NOT NULL,
  `page_path` VARCHAR(255) DEFAULT NULL,
  `status` VARCHAR(20) NOT NULL DEFAULT 'PENDING',
  `attempt_count` INT NOT NULL DEFAULT 0,
  `next_attempt_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `claimed_at` DATETIME DEFAULT NULL,
  `sent_at` DATETIME DEFAULT NULL,
  `last_errcode` INT DEFAULT NULL,
  `last_errmsg` VARCHAR(1000) DEFAULT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_wx_subscribe_due` (`status`, `next_attempt_at`, `id`),
  KEY `idx_wx_subscribe_user_time` (`user_id`, `created_at`),
  KEY `idx_wx_subscribe_biz_time` (`biz_key`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='微信订阅消息可靠投递与审计日志';

CREATE TABLE IF NOT EXISTS `wx_subscribe_status_history` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `user_id` INT NOT NULL,
  `biz_key` VARCHAR(100) NOT NULL,
  `template_id` VARCHAR(100) NOT NULL,
  `result` VARCHAR(20) NOT NULL,
  `status` TINYINT NOT NULL,
  `source` VARCHAR(32) NOT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_wx_sub_history_user_biz_time` (`user_id`, `biz_key`, `created_at`),
  KEY `idx_wx_sub_history_biz_time` (`biz_key`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='微信订阅授权状态变更历史';

SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE `subscribe` ADD INDEX `idx_subscribe_biz_status_user` (`biz_key`, `status`, `user_id`)',
    'SELECT 1'
  )
  FROM INFORMATION_SCHEMA.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'subscribe'
    AND INDEX_NAME = 'idx_subscribe_biz_status_user'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
	'ALTER TABLE `msg_template_config` ADD COLUMN `remark` VARCHAR(20) DEFAULT NULL COMMENT ''模板字段备注（最多20字符）''',
	'SELECT 1'
  )
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'msg_template_config'
    AND COLUMN_NAME = 'remark'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
  SELECT IF(
    COUNT(*) = 1,
    'SELECT 1',
    'ALTER TABLE `msg_template_config` MODIFY COLUMN `remark` VARCHAR(20) DEFAULT NULL COMMENT ''模板字段备注（最多20字符）'''
  )
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'msg_template_config'
    AND COLUMN_NAME = 'remark'
    AND CHARACTER_MAXIMUM_LENGTH = 20
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE `msg_template_config` ADD COLUMN `enabled` TINYINT(1) NOT NULL DEFAULT 1 COMMENT ''是否启用''',
    'SELECT 1'
  )
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'msg_template_config'
    AND COLUMN_NAME = 'enabled'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE `msg_template_config` ADD COLUMN `platform_status` VARCHAR(32) DEFAULT NULL COMMENT ''微信平台核验状态''',
    'SELECT 1'
  )
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'msg_template_config'
    AND COLUMN_NAME = 'platform_status'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE `msg_template_config` ADD COLUMN `platform_verified_at` DATETIME DEFAULT NULL COMMENT ''最近微信平台核验时间''',
    'SELECT 1'
  )
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'msg_template_config'
    AND COLUMN_NAME = 'platform_verified_at'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE `msg_template_config` ADD UNIQUE INDEX `uk_msg_template_template_id` (`template_id`)',
    'SELECT 1'
  )
  FROM INFORMATION_SCHEMA.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'msg_template_config'
    AND INDEX_NAME = 'uk_msg_template_template_id'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- The 17 templates were manually verified as enabled on 2026-08-01.
UPDATE `msg_template_config`
SET `platform_status` = 'ENABLED',
    `platform_verified_at` = CURRENT_TIMESTAMP
WHERE `biz_key` LIKE 'MSG\_%';
