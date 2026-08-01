-- 只读验证：执行 20260801_message_task_indexes.sql 后运行。
SELECT table_name, index_name, seq_in_index, column_name, non_unique
FROM information_schema.statistics
WHERE table_schema = DATABASE()
  AND index_name IN (
    'uk_email_task_task_key',
    'idx_pa_user_status',
    'idx_ob_receiver_status_created',
    'idx_ob_sender_updated'
  )
ORDER BY table_name, index_name, seq_in_index;

SELECT task_key, COUNT(*) AS duplicate_count
FROM email_task
WHERE task_key IS NOT NULL AND task_key <> ''
GROUP BY task_key
HAVING COUNT(*) > 1;
