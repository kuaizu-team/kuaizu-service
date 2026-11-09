-- MySQL 5.7+。先执行本文件，再部署读取 event.tags 的后端。
-- 仅增加可空 JSON 字段，不改动已有赛事内容，可重复执行。
SELECT COUNT(*) INTO @event_tags_exists
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'event' AND COLUMN_NAME = 'tags';
SET @event_tags_ddl := IF(@event_tags_exists = 0,
  'ALTER TABLE event ADD COLUMN tags JSON NULL COMMENT ''赛事特色标签，最多三个'' AFTER summary',
  'SELECT ''event.tags already exists'' AS message');
PREPARE event_tags_stmt FROM @event_tags_ddl;
EXECUTE event_tags_stmt;
DEALLOCATE PREPARE event_tags_stmt;

SELECT COLUMN_NAME,COLUMN_TYPE FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='event' AND COLUMN_NAME='tags';
