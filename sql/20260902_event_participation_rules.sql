-- Event cross-school/major and participation rules (MySQL 8+, repeatable).
-- Run manually before deploying the corresponding backend version.

DROP PROCEDURE IF EXISTS add_event_participation_column;
DELIMITER $$
CREATE PROCEDURE add_event_participation_column(IN column_name_value VARCHAR(64), IN column_definition_value TEXT)
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'event' AND COLUMN_NAME = column_name_value
  ) THEN
    SET @event_participation_ddl = CONCAT('ALTER TABLE `event` ADD COLUMN `', column_name_value, '` ', column_definition_value);
    PREPARE event_participation_stmt FROM @event_participation_ddl;
    EXECUTE event_participation_stmt;
    DEALLOCATE PREPARE event_participation_stmt;
  END IF;
END$$
DELIMITER ;

CALL add_event_participation_column('cross_school_major_rule', 'VARCHAR(32) NULL COMMENT ''跨校跨专业综合规则'' AFTER `allow_cross_major`');
CALL add_event_participation_column('participation_mode', 'VARCHAR(16) NULL COMMENT ''参赛形式：individual/team'' AFTER `cross_school_major_rule`');
CALL add_event_participation_column('team_min_members', 'INT UNSIGNED NULL COMMENT ''团队最少人数'' AFTER `participation_mode`');
CALL add_event_participation_column('team_max_members', 'INT UNSIGNED NULL COMMENT ''团队最多人数'' AFTER `team_min_members`');

DROP PROCEDURE IF EXISTS add_event_participation_column;

-- Old boolean fields map losslessly to the three new choices.
UPDATE `event`
SET `cross_school_major_rule` = CASE
  WHEN COALESCE(`allow_cross_major`, 1) = 0 THEN 'reject_cross_major'
  WHEN COALESCE(`allow_cross_school`, 1) = 0 THEN 'allow_cross_major'
  ELSE 'allow_cross_school_and_major'
END
WHERE `cross_school_major_rule` IS NULL OR `cross_school_major_rule` = '';

-- Participation stays NULL for legacy rows because it cannot be inferred safely.
UPDATE `event`
SET `team_min_members` = NULL, `team_max_members` = NULL
WHERE `participation_mode` IS NULL OR `participation_mode` = 'individual';
