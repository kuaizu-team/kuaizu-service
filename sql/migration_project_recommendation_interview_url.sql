-- Add interview link for project recommendations.
-- MySQL 5.7 compatible: run once; it checks information_schema before ALTER TABLE.

SET @column_exists := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'project_recommendation'
    AND COLUMN_NAME = 'interview_url'
);

SET @ddl := IF(
  @column_exists = 0,
  'ALTER TABLE project_recommendation ADD COLUMN interview_url VARCHAR(500) NULL COMMENT ''采访链接'' AFTER is_featured',
  'SELECT ''project_recommendation.interview_url already exists'' AS message'
);

PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;