-- 赛事总报名截止节点清理归档（人工发布脚本，不纳入服务启动迁移）。
-- 已于 2026-09-05 在 kuaizu_db 完成正式执行并验证：
--   备份 144 条、删除 144 条、标准重复节点剩余 0 条；
--   6 条阶段性报名截止保留，event.registration_deadline 非空仍为 144 条。
-- 常规部署不要重跑本脚本，也不要重跑旧迁移中插入“报名截止”的回填段。
-- 如需在其他环境执行，必须先部署兼容代码并完成只读审计和数据库备份。
-- 默认以 ROLLBACK 结束；仅在人工核对 deleted_standard_deadline_nodes 后改为 COMMIT。

SELECT DATABASE() AS target_database;

-- 仅处理已审计的节点 ID 1..144；未来新增节点必须另行审计。
-- 不涉及团队/意向/决赛/校赛/网络报名截止等阶段节点；
-- 不修改 event.registration_deadline。
CREATE TABLE IF NOT EXISTS event_deadline_backup_20260904
LIKE event_timeline_node;

START TRANSACTION;

INSERT INTO event_deadline_backup_20260904
  (id, event_id, title, node_time, time_text, description,
   sort_order, created_at, updated_at)
SELECT
  n.id,
  n.event_id,
  n.title,
  n.node_time,
  n.time_text,
  n.description,
  n.sort_order,
  n.created_at,
  n.updated_at
FROM event_timeline_node n
JOIN `event` e ON e.id = n.event_id
WHERE n.id BETWEEN 1 AND 144
  AND n.title = '报名截止'
  AND DATE(n.node_time) = e.registration_deadline
  AND TIME(n.node_time) = '23:59:00'
  AND NULLIF(TRIM(n.time_text), '') IS NULL
  AND NULLIF(TRIM(n.description), '') IS NULL
  AND NOT EXISTS (
    SELECT 1
    FROM event_deadline_backup_20260904 b
    WHERE b.id = n.id
  );

SELECT ROW_COUNT() AS backed_up_standard_deadline_nodes;

DELETE n
FROM event_timeline_node n
JOIN `event` e ON e.id = n.event_id
JOIN event_deadline_backup_20260904 b ON b.id = n.id
WHERE n.id BETWEEN 1 AND 144
  AND n.title = '报名截止'
  AND DATE(n.node_time) = e.registration_deadline
  AND TIME(n.node_time) = '23:59:00'
  AND NULLIF(TRIM(n.time_text), '') IS NULL
  AND NULLIF(TRIM(n.description), '') IS NULL
  AND n.event_id = b.event_id
  AND BINARY n.title = BINARY b.title
  AND n.node_time <=> b.node_time
  AND n.time_text <=> b.time_text
  AND n.description <=> b.description
  AND n.sort_order = b.sort_order
  AND n.created_at = b.created_at
  AND n.updated_at = b.updated_at;

SELECT ROW_COUNT() AS deleted_standard_deadline_nodes;

-- 安全默认：演练回滚。其他环境正式执行时经人工确认后改为 COMMIT。
ROLLBACK;

-- 正式提交后的只读验收：
-- SELECT COUNT(*) AS remaining_standard_nodes
-- FROM event_timeline_node
-- WHERE id BETWEEN 1 AND 144 AND title = '报名截止';
--
-- SELECT COUNT(*) AS backup_count
-- FROM event_deadline_backup_20260904;
--
-- SELECT COUNT(*) AS remaining_stage_deadlines
-- FROM event_timeline_node
-- WHERE title LIKE '%报名%截止%'
--   AND TRIM(title) NOT IN ('报名截止', '报名截止时间');
--
-- SELECT COUNT(*) AS independent_deadlines
-- FROM `event`
-- WHERE registration_deadline IS NOT NULL;

-- 恢复模板（停写并确认节点 ID 未被复用后，人工单独执行）：
-- START TRANSACTION;
-- INSERT INTO event_timeline_node
--   (id, event_id, title, node_time, time_text, description,
--    sort_order, created_at, updated_at)
-- SELECT
--   b.id, b.event_id, b.title, b.node_time, b.time_text, b.description,
--   b.sort_order, b.created_at, b.updated_at
-- FROM event_deadline_backup_20260904 b
-- JOIN `event` e ON e.id = b.event_id
-- WHERE NOT EXISTS (
--   SELECT 1 FROM event_timeline_node n WHERE n.id = b.id
-- );
-- COMMIT;
