CREATE TABLE IF NOT EXISTS `pending_invitation` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_id` int(11) NOT NULL COMMENT 'user id',
  `invite_type` enum('SUPER_ADMIN') NOT NULL COMMENT 'invitation type',
  `expire_at` timestamp NOT NULL COMMENT 'expire time',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'created time',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'updated time',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_pending_invitation_user_type` (`user_id`,`invite_type`),
  KEY `idx_pending_invitation_expire_at` (`expire_at`),
  CONSTRAINT `fk_pending_invitation_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='pending invitation display flag';
