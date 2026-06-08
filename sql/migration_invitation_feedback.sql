CREATE TABLE IF NOT EXISTS `invitation_feedback` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_id` int(11) NOT NULL COMMENT 'user id',
  `status` enum('pending','interested','not_interested') NOT NULL DEFAULT 'pending' COMMENT 'feedback status',
  `intention_text` varchar(500) DEFAULT NULL COMMENT 'intention text max 500 chars',
  `conversation_status` enum('in_progress','accepted','rejected') DEFAULT NULL COMMENT 'conversation status',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'created time',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'updated time',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_invitation_feedback_user` (`user_id`),
  KEY `idx_invitation_feedback_status` (`status`),
  KEY `idx_invitation_feedback_conversation_status` (`conversation_status`),
  CONSTRAINT `fk_invitation_feedback_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='campus super admin invitation feedback';
