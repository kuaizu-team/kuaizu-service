-- MySQL 5.7: pending status pages owned by the affected user.
CREATE TABLE IF NOT EXISTS `status_notification` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `user_id` INT NOT NULL,
  `type` VARCHAR(50) NOT NULL,
  `application_id` INT NOT NULL,
  `displayed_at` TIMESTAMP NULL DEFAULT NULL,
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_status_notification_pending` (`user_id`, `displayed_at`, `id`),
  KEY `idx_status_notification_application` (`application_id`),
  CONSTRAINT `fk_status_notification_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_status_notification_application` FOREIGN KEY (`application_id`) REFERENCES `project_application` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Status notification delivery queue';
