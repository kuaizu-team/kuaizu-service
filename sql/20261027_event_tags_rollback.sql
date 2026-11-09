-- 仅回滚本次补齐且仍与补齐值相同的标签；后续人工修改的标签保留。
-- 不删除 event.tags 字段，也不删除已共享使用的项目标签库条目。
START TRANSACTION;
UPDATE event e JOIN event_tags_backup_20261027 b ON e.id=b.event_id
SET e.tags=b.tags,e.updated_at=e.updated_at
WHERE e.tags=b.applied_tags;
SELECT ROW_COUNT() AS events_restored;
COMMIT;
