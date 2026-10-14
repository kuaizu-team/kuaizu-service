-- Step 3: READ ONLY, after 20261008_feishu_user_sync.sql.
-- Expect all three tables, PKs, and the unique active-school/client-token indexes.
SELECT DATABASE() AS selected_database;
SELECT table_name,engine,table_collation
FROM information_schema.tables
WHERE table_schema=DATABASE() AND table_name IN
 ('feishu_user_sync_target','feishu_user_sync_record','feishu_user_sync_job')
ORDER BY table_name;

SELECT table_name,column_name,column_type,is_nullable,column_default
FROM information_schema.columns
WHERE table_schema=DATABASE() AND table_name IN
 ('feishu_user_sync_target','feishu_user_sync_record','feishu_user_sync_job')
ORDER BY table_name,ordinal_position;

SELECT table_name,index_name,non_unique,seq_in_index,column_name
FROM information_schema.statistics
WHERE table_schema=DATABASE() AND table_name IN
 ('feishu_user_sync_target','feishu_user_sync_record','feishu_user_sync_job')
ORDER BY table_name,index_name,seq_in_index;

-- Fresh installation should have zero rows. Do not truncate existing rows.
SELECT 'feishu_user_sync_target' AS sync_table,COUNT(*) AS row_count FROM feishu_user_sync_target
UNION ALL SELECT 'feishu_user_sync_record',COUNT(*) FROM feishu_user_sync_record
UNION ALL SELECT 'feishu_user_sync_job',COUNT(*) FROM feishu_user_sync_job;
