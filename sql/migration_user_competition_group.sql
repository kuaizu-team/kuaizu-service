CREATE TABLE IF NOT EXISTS `user_competition_group` (
  `user_id` INT NOT NULL COMMENT '用户ID',
  `status` VARCHAR(20) DEFAULT NULL COMMENT '比赛交流群状态：entered/rejected',
  `note` TEXT DEFAULT NULL COMMENT '比赛交流群备注',
  `updated_by_admin_id` INT DEFAULT NULL COMMENT '最后更新管理员ID',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`user_id`),
  KEY `idx_user_competition_group_status` (`status`),
  KEY `idx_user_competition_group_updated_by` (`updated_by_admin_id`),
  CONSTRAINT `fk_user_competition_group_user` FOREIGN KEY (`user_id`) REFERENCES `user` (`id`) ON DELETE CASCADE,
  CONSTRAINT `fk_user_competition_group_admin` FOREIGN KEY (`updated_by_admin_id`) REFERENCES `admin_user` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户比赛交流群状态';