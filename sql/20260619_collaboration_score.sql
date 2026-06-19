ALTER TABLE `user`
  ADD COLUMN `collaboration_score` DECIMAL(5,2) NOT NULL DEFAULT 100.00 COMMENT '协作指数，0-100分' AFTER `ban_reason`;

CREATE TABLE IF NOT EXISTS `collaboration_score` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id` INT UNSIGNED NOT NULL COMMENT '被评用户ID',
  `project_id` INT UNSIGNED NOT NULL COMMENT '关联项目ID',
  `scorer_id` INT UNSIGNED NOT NULL COMMENT '评分人ID',
  `score` TINYINT UNSIGNED NOT NULL COMMENT '0-100分',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_collaboration_score_user` (`user_id`),
  KEY `idx_collaboration_score_project` (`project_id`),
  KEY `idx_collaboration_score_scorer` (`scorer_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='协作指数评分记录';

UPDATE `user`
SET `collaboration_score` = 100.00
WHERE `collaboration_score` IS NULL;
