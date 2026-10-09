-- Step 1: READ ONLY. Select the intended database in your SQL client first.
-- No user data, configuration, records, or schema are changed.
SELECT DATABASE() AS selected_database, VERSION() AS mysql_version;

-- This result should be empty. Missing source columns must be resolved before sync.
SELECT required_columns.table_name, required_columns.column_name AS missing_column
FROM (
  SELECT 'user' table_name, 'id' column_name UNION ALL
  SELECT 'user','school_id' UNION ALL SELECT 'user','major_id' UNION ALL
  SELECT 'user','nickname' UNION ALL SELECT 'user','grade' UNION ALL
  SELECT 'user','collaboration_score' UNION ALL SELECT 'user','auth_status' UNION ALL
  SELECT 'user','phone' UNION ALL SELECT 'user','wechat_id' UNION ALL
  SELECT 'user','email' UNION ALL SELECT 'user','user_status' UNION ALL
  SELECT 'talent_profile','user_id' UNION ALL SELECT 'talent_profile','mbti' UNION ALL
  SELECT 'talent_profile','self_evaluation' UNION ALL SELECT 'talent_profile','project_experience' UNION ALL
  SELECT 'talent_profile','status' UNION ALL
  SELECT 'school','id' UNION ALL SELECT 'school','school_name' UNION ALL
  SELECT 'major','id' UNION ALL SELECT 'major','major_name' UNION ALL
  SELECT 'admin_user','id' UNION ALL SELECT 'admin_user','role' UNION ALL
  SELECT 'admin_user','school_id' UNION ALL SELECT 'admin_user','status' UNION ALL
  SELECT 'admin_school_relation','admin_user_id' UNION ALL
  SELECT 'admin_school_relation','school_id' UNION ALL SELECT 'admin_school_relation','commission_rate'
) required_columns
LEFT JOIN information_schema.columns actual
  ON actual.table_schema=DATABASE()
 AND actual.table_name=required_columns.table_name
 AND actual.column_name=required_columns.column_name
WHERE actual.column_name IS NULL
ORDER BY required_columns.table_name,required_columns.column_name;

-- Existing sync tables (normally zero before first migration).
SELECT table_name FROM information_schema.tables
WHERE table_schema=DATABASE() AND table_name IN
 ('feishu_user_sync_target','feishu_user_sync_record','feishu_user_sync_job');

-- Aggregate only: no nicknames, phones, email addresses or user profiles are returned.
SELECT COUNT(*) AS total_users,
       SUM(school_id IS NULL OR school_id<=0) AS excluded_unbound_users
FROM `user`;
SELECT COUNT(*) AS users_with_invalid_school_reference
FROM `user` u LEFT JOIN school s ON s.id=u.school_id
WHERE u.school_id>0 AND s.id IS NULL;

-- This result should be empty; one talent profile per user is required.
SELECT user_id,COUNT(*) AS duplicate_profiles
FROM talent_profile GROUP BY user_id HAVING COUNT(*)>1 LIMIT 20;

-- Index metadata: confirm a school lookup index and a UNIQUE talent_profile.user_id.
SELECT table_name,index_name,non_unique,seq_in_index,column_name
FROM information_schema.statistics
WHERE table_schema=DATABASE() AND table_name IN ('user','talent_profile')
ORDER BY table_name,index_name,seq_in_index;

-- Small schools are candidates for the later first trial. This does not authorize a sync.
SELECT s.id AS school_id,s.school_name,COUNT(*) AS user_count
FROM school s JOIN `user` u ON u.school_id=s.id
GROUP BY s.id,s.school_name HAVING COUNT(*) BETWEEN 2 AND 20
ORDER BY user_count,s.id LIMIT 10;

SELECT role,status,COUNT(*) AS administrators,
       SUM(school_id IS NULL OR school_id<=0) AS unbound_administrators
FROM admin_user GROUP BY role,status ORDER BY role,status;
