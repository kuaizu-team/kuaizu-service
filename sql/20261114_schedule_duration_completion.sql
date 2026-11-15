-- 日程起止时间与完成状态。执行前备份；需先执行 user_schedule 和 schedule_import 迁移。
-- 仅执行一次。旧日程的结束时间保留 NULL，不猜填；完成状态默认为未完成。
ALTER TABLE user_schedule
  ADD COLUMN end_date DATE DEFAULT NULL AFTER schedule_time,
  ADD COLUMN end_time TIME DEFAULT NULL AFTER end_date,
  ADD COLUMN completed TINYINT(1) NOT NULL DEFAULT 0 AFTER end_time;

-- 核对 end_date/date、end_time/time（允许 NULL），completed/tinyint（默认 0）。
SHOW COLUMNS FROM user_schedule;
-- 发布顺序：执行 SQL → 部署后端 → 发布小程序。已有索引不变。
