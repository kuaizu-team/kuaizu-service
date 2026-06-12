-- Register invite SMS record migration for MySQL 5.7+.
-- Records project-team invitations sent to phones that are not registered yet.

CREATE TABLE IF NOT EXISTS `invitation_record` (
  `id` int(11) NOT NULL AUTO_INCREMENT COMMENT 'primary key',
  `phone` varchar(20) NOT NULL COMMENT 'invited phone number',
  `project_id` int(11) NOT NULL COMMENT 'project id',
  `inviter_user_id` int(11) NOT NULL COMMENT 'user who sent the invite',
  `role` varchar(50) NOT NULL COMMENT 'project role code',
  `status` varchar(20) NOT NULL DEFAULT 'SENT' COMMENT 'SENT/FAILED/REGISTERED/JOINED',
  `provider` varchar(50) DEFAULT NULL COMMENT 'sms provider',
  `provider_biz_id` varchar(128) DEFAULT NULL COMMENT 'provider business id',
  `error_message` text COMMENT 'last send error',
  `sent_at` datetime DEFAULT NULL COMMENT 'sms sent time',
  `registered_at` datetime DEFAULT NULL COMMENT 'reserved: phone registered time',
  `joined_at` datetime DEFAULT NULL COMMENT 'reserved: joined project time',
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'created time',
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'updated time',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_invitation_record_phone_project` (`phone`,`project_id`),
  KEY `idx_invitation_record_project` (`project_id`),
  KEY `idx_invitation_record_inviter` (`inviter_user_id`),
  KEY `idx_invitation_record_status` (`status`),
  CONSTRAINT `fk_invitation_record_project` FOREIGN KEY (`project_id`) REFERENCES `project` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_invitation_record_inviter` FOREIGN KEY (`inviter_user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='project team register invitation records';
