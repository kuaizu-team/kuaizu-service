-- Optional cleanup. Execute only after every service instance has been upgraded
-- to the code that no longer reads or writes subscribe.subscribe_count.
SET @sql := (
  SELECT IF(
    COUNT(*) = 1,
    'ALTER TABLE `subscribe` DROP COLUMN `subscribe_count`',
    'SELECT 1'
  )
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'subscribe'
    AND COLUMN_NAME = 'subscribe_count'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
