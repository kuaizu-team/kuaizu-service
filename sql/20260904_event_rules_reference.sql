-- 01: Schema only. Export event and event_timeline_node before running. MySQL 5.7+/8.0.
-- DDL auto-commits. Run before deploying the backend; do NOT run data backfill until backend is upgraded.
SET NAMES utf8mb4;
SET @event_rules_ddl = IF(EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='event' AND column_name='participation_note'), 'SELECT 1', 'ALTER TABLE `event` ADD COLUMN `participation_note` TEXT NULL COMMENT ''知识库参赛方式和学生人数说明'' AFTER `participation_mode`');
PREPARE event_rules_stmt FROM @event_rules_ddl; EXECUTE event_rules_stmt; DEALLOCATE PREPARE event_rules_stmt;
SET @event_rules_ddl = IF(EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='event_timeline_node' AND column_name='time_text'), 'SELECT 1', 'ALTER TABLE `event_timeline_node` ADD COLUMN `time_text` VARCHAR(500) NULL COMMENT ''原文时间说明；无具体日期时按阶段顺序展示'' AFTER `node_time`');
PREPARE event_rules_stmt FROM @event_rules_ddl; EXECUTE event_rules_stmt; DEALLOCATE PREPARE event_rules_stmt;
ALTER TABLE `event_timeline_node` MODIFY COLUMN `node_time` DATETIME NULL;
SELECT table_name,column_name,column_type,is_nullable FROM information_schema.columns WHERE table_schema=DATABASE() AND ((table_name='event' AND column_name='participation_note') OR (table_name='event_timeline_node' AND column_name IN ('node_time','time_text')));
