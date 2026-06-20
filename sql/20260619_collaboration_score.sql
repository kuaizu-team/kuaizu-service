-- Collaboration score migration for MySQL 5.7+.
-- Safe to re-run. MySQL 5.7 does not support ADD COLUMN IF NOT EXISTS.

DROP PROCEDURE IF EXISTS _collaboration_score_add_user_column;
DELIMITER $$
CREATE PROCEDURE _collaboration_score_add_user_column()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'user'
      AND column_name = 'collaboration_score'
  ) THEN
    ALTER TABLE `user`
      ADD COLUMN `collaboration_score` DECIMAL(5,2) NOT NULL DEFAULT 100.00 COMMENT 'collaboration score, 0-100';
  END IF;
END$$
DELIMITER ;
CALL _collaboration_score_add_user_column();
DROP PROCEDURE IF EXISTS _collaboration_score_add_user_column;

CREATE TABLE IF NOT EXISTS `collaboration_score` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id` INT UNSIGNED NOT NULL COMMENT 'rated user ID',
  `project_id` INT UNSIGNED NOT NULL COMMENT 'project ID',
  `scorer_id` INT UNSIGNED NOT NULL COMMENT 'scorer user ID',
  `score` TINYINT UNSIGNED NOT NULL COMMENT '0-100 score',
  `created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_collaboration_score_user` (`user_id`),
  KEY `idx_collaboration_score_project` (`project_id`),
  KEY `idx_collaboration_score_scorer` (`scorer_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='collaboration score history';

UPDATE `user`
SET `collaboration_score` = 100.00
WHERE `collaboration_score` IS NULL;
