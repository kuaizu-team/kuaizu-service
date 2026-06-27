-- Add stable event timestamps for status notification pages.
-- Compatible with MySQL 5.7: each ADD COLUMN is guarded by information_schema checks.

SET @schema_name := DATABASE();

SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE project_application ADD COLUMN discussing_at TIMESTAMP NULL DEFAULT NULL COMMENT ''Discussing status timestamp'' AFTER applied_at',
    'SELECT 1'
  )
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @schema_name
    AND TABLE_NAME = 'project_application'
    AND COLUMN_NAME = 'discussing_at'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE project_application ADD COLUMN rejected_at TIMESTAMP NULL DEFAULT NULL COMMENT ''Rejected timestamp'' AFTER discussing_at',
    'SELECT 1'
  )
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @schema_name
    AND TABLE_NAME = 'project_application'
    AND COLUMN_NAME = 'rejected_at'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE project_application ADD COLUMN joined_at TIMESTAMP NULL DEFAULT NULL COMMENT ''Joined/admitted timestamp'' AFTER rejected_at',
    'SELECT 1'
  )
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @schema_name
    AND TABLE_NAME = 'project_application'
    AND COLUMN_NAME = 'joined_at'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE project ADD COLUMN recruit_completed_at TIMESTAMP NULL DEFAULT NULL COMMENT ''Recruit completed timestamp'' AFTER passive_status_changed_at',
    'SELECT 1'
  )
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @schema_name
    AND TABLE_NAME = 'project'
    AND COLUMN_NAME = 'recruit_completed_at'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql := (
  SELECT IF(
    COUNT(*) = 0,
    'ALTER TABLE project ADD COLUMN ended_at TIMESTAMP NULL DEFAULT NULL COMMENT ''Project ended timestamp'' AFTER recruit_completed_at',
    'SELECT 1'
  )
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = @schema_name
    AND TABLE_NAME = 'project'
    AND COLUMN_NAME = 'ended_at'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Backfill approximate historical timestamps so existing records can render meaningful notices.
UPDATE project_application
SET discussing_at = updated_at
WHERE status IN (1, 3)
  AND discussing_at IS NULL;

UPDATE project_application
SET rejected_at = updated_at
WHERE status = 2
  AND rejected_at IS NULL;

UPDATE project_application
SET joined_at = updated_at
WHERE status = 3
  AND joined_at IS NULL;

UPDATE project
SET recruit_completed_at = updated_at
WHERE status = 3
  AND recruit_completed_at IS NULL;

UPDATE project
SET ended_at = updated_at
WHERE status = 5
  AND ended_at IS NULL;