-- Welcome-email delivery history. Compatible with MySQL 5.7.
-- The same recipient_email may appear multiple times: every genuine email
-- change triggers a new welcome message regardless of historical deliveries.
CREATE TABLE IF NOT EXISTS `welcome_email_delivery` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `recipient_email` VARCHAR(255) NOT NULL COMMENT 'recipient email for this delivery',
  `user_id` INT DEFAULT NULL COMMENT 'user who triggered this delivery',
  `status` ENUM('pending','sent','failed') NOT NULL DEFAULT 'pending',
  `message_task_id` BIGINT DEFAULT NULL COMMENT 'message-center email_task.id after acceptance',
  `error_message` VARCHAR(500) DEFAULT NULL,
  `sent_at` DATETIME DEFAULT NULL,
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_welcome_email_recipient` (`recipient_email`),
  KEY `idx_welcome_email_user_id` (`user_id`),
  KEY `idx_welcome_email_status` (`status`),
  CONSTRAINT `fk_welcome_email_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Welcome email delivery history';
