-- 赛事简称字段。可重复执行，不修改现有赛事数据。
SELECT COUNT(*) INTO @event_short_name_exists
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'event'
  AND COLUMN_NAME = 'short_name';

SET @event_short_name_ddl := IF(
  @event_short_name_exists = 0,
  'ALTER TABLE `event` ADD COLUMN `short_name` VARCHAR(64) NULL COMMENT ''赛事简称/俗称'' AFTER `name`',
  'SELECT ''event.short_name already exists'' AS message'
);

PREPARE event_short_name_stmt FROM @event_short_name_ddl;
EXECUTE event_short_name_stmt;
DEALLOCATE PREPARE event_short_name_stmt;

SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_COMMENT
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'event'
  AND COLUMN_NAME = 'short_name';
