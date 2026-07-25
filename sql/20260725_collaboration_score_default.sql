-- Change the initial collaboration score without affecting rated users.

ALTER TABLE `user`
  MODIFY COLUMN `collaboration_score` DECIMAL(5,2) NOT NULL DEFAULT 90.00
  COMMENT 'collaboration score, 0-100';

UPDATE `user` u
SET u.`collaboration_score` = 90.00
WHERE u.`collaboration_score` = 100.00
  AND NOT EXISTS (
    SELECT 1
    FROM `collaboration_score` cs
    WHERE cs.`user_id` = u.`id`
  );
