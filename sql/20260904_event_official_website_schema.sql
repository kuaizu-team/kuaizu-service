-- 阶段一：人工审核后执行。不会自动连接或修改数据库。
-- 请先选择正确业务数据库并备份 event/event_timeline_node。
-- MySQL PREPARE；DDL 隐式提交，不能用 ROLLBACK 撤销。
-- 导出为 INSERT-only，不含线上 DDL，因此先核对当前库及字段结构。
SELECT DATABASE() AS target_database;
SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'event'
  AND COLUMN_NAME IN ('official_website', 'registration_deadline');

SET @event_website_ddl = IF(
  EXISTS (SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'event'
      AND COLUMN_NAME = 'official_website'),
  'SELECT ''official_website already exists; verify its definition'' AS migration_status',
  'ALTER TABLE `event` ADD COLUMN `official_website` VARCHAR(2048) NULL DEFAULT NULL COMMENT ''赛事官网链接'''
);
PREPARE event_website_stmt FROM @event_website_ddl;
EXECUTE event_website_stmt;
DEALLOCATE PREPARE event_website_stmt;

-- registration_deadline 已存在，现有契约为 DATE NULL；不要另建重复字段。
-- 144 个标准报名截止节点全部与独立字段同日，回填记录数为 0。
-- 不用公众号 article_url 或资料池 resource_url 猜测官网地址。
SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'event'
  AND COLUMN_NAME IN ('official_website', 'registration_deadline');
