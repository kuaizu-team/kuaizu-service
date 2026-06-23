-- Add custom card description for project recommendations.
-- MySQL 5.7 compatible: run once; it checks information_schema before ALTER TABLE.

SET @column_exists := (
  SELECT COUNT(*)
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'project_recommendation'
    AND COLUMN_NAME = 'description'
);

SET @ddl := IF(
  @column_exists = 0,
  'ALTER TABLE project_recommendation ADD COLUMN description VARCHAR(500) NULL COMMENT ''卡片描述'' AFTER project_id',
  'SELECT ''project_recommendation.description already exists'' AS message'
);

PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;