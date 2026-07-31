-- Add the time-leading index used by the seven-day delivery overview.
-- This migration is separate because the delivery table may already exist.
SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE `wx_subscribe_delivery` ADD INDEX `idx_wx_subscribe_created_status` (`created_at`, `status`)',
    'SELECT 1'
  )
  FROM INFORMATION_SCHEMA.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'wx_subscribe_delivery'
    AND INDEX_NAME = 'idx_wx_subscribe_created_status'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SELECT INDEX_NAME,
       GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX) AS indexed_columns
FROM INFORMATION_SCHEMA.STATISTICS
WHERE TABLE_SCHEMA = DATABASE()
  AND TABLE_NAME = 'wx_subscribe_delivery'
  AND INDEX_NAME = 'idx_wx_subscribe_created_status'
GROUP BY INDEX_NAME;
