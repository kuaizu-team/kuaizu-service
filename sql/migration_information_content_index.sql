-- MySQL 5.7 compatible index migration for information_content.
-- Speeds up public category lists and admin category filtering.

SET @idx_exists := (
    SELECT COUNT(*)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'information_content'
      AND index_name = 'idx_information_content_category_publish_order'
);

SET @sql := IF(
    @idx_exists = 0,
    'ALTER TABLE information_content ADD INDEX idx_information_content_category_publish_order (category, is_published, display_order, created_at, id)',
    'SELECT 1'
);

PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
