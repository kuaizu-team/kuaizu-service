-- Read-only index gate. Expected result: 7/7/PASS and 0 removed indexes.

SELECT
  7 AS expected_index_count,
  COUNT(*) AS passed_index_count,
  CASE WHEN COUNT(*) = 7 THEN 'PASS' ELSE 'FAIL' END AS verification_status
FROM (
  SELECT table_name, index_name,
         GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ',') AS indexed_columns
  FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND (table_name, index_name) IN (
      ('project_view_log', 'idx_pvl_project_duration_viewed_user'),
      ('project_view_log', 'idx_pvl_project_user_duration_viewed'),
      ('talent_view_log', 'idx_tvl_talent_duration_viewed_user'),
      ('talent_view_log', 'idx_tvl_talent_user_duration_viewed'),
      ('email_task', 'idx_email_task_promotion_channel_status'),
      ('email_task', 'idx_email_task_recipient_promotion'),
      ('email_promotion', 'idx_ep_reconcile')
    )
  GROUP BY table_name, index_name
) actual
JOIN (
  SELECT 'project_view_log' AS table_name, 'idx_pvl_project_duration_viewed_user' AS index_name, 'project_id,duration_ms,viewed_at,user_id,id' AS indexed_columns
  UNION ALL SELECT 'project_view_log', 'idx_pvl_project_user_duration_viewed', 'project_id,user_id,duration_ms,viewed_at,id'
  UNION ALL SELECT 'talent_view_log', 'idx_tvl_talent_duration_viewed_user', 'talent_id,duration_ms,viewed_at,user_id,id'
  UNION ALL SELECT 'talent_view_log', 'idx_tvl_talent_user_duration_viewed', 'talent_id,user_id,duration_ms,viewed_at,id'
  UNION ALL SELECT 'email_task', 'idx_email_task_promotion_channel_status', 'promotion_id,channel,status'
  UNION ALL SELECT 'email_task', 'idx_email_task_recipient_promotion', 'recipient_email,promotion_id'
  UNION ALL SELECT 'email_promotion', 'idx_ep_reconcile', 'channel,business_tag,status,id'
) expected USING (table_name, index_name, indexed_columns);

SELECT COUNT(DISTINCT CONCAT(table_name, ':', index_name)) AS remaining_removed_indexes
FROM information_schema.statistics
WHERE table_schema = DATABASE()
  AND (table_name, index_name) IN (
    ('project_view_log', 'idx_pvl_project_viewed_at'),
    ('project_view_log', 'idx_pvl_project_user'),
    ('email_task', 'idx_promotion_id'),
    ('admin_user', 'idx_admin_user_username'),
    ('talent_profile', 'idx_talent_user'),
    ('user', 'idx_user_openid')
  );
