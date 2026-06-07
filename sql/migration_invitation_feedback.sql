CREATE TABLE IF NOT EXISTS `invitation_feedback` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'ID',
  `user_id` int(11) NOT NULL COMMENT '用户ID',
  `status` enum('pending','interested','not_interested') NOT NULL DEFAULT 'pending' COMMENT '反馈状态: pending-待定, interested-感兴趣, not_interested-不感兴趣',
  `intention_text` varchar(500) DEFAULT NULL COMMENT '意向文本，最多500字',
  `conversation_status` enum('in_progress','accepted','rejected') DEFAULT NULL COMMENT '对接状态: in_progress-正在聊, accepted-已加入, rejected-已拒绝',
  `created_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_invitation_feedback_user` (`user_id`),
  KEY `idx_invitation_feedback_status` (`status`),
  KEY `idx_invitation_feedback_conversation_status` (`conversation_status`),
  CONSTRAINT `fk_invitation_feedback_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='校区超级管理员邀请反馈表';
