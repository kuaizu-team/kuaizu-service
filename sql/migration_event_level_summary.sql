-- MySQL 5.7 compatible. Run once against the kuaizu database.
ALTER TABLE `event`
  ADD COLUMN `level` VARCHAR(20) NULL COMMENT '赛事等级: national/regional/school' AFTER `article_url`,
  ADD COLUMN `summary` VARCHAR(255) NULL COMMENT '赛事一句话描述' AFTER `level`,
  ADD COLUMN `school_id` INT NULL COMMENT '学校级赛事所属学校ID' AFTER `summary`,
  ADD INDEX `idx_event_level` (`level`),
  ADD INDEX `idx_event_school_id` (`school_id`);
