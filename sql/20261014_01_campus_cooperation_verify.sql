-- READ ONLY. Run before/after the companion migration and return these structural results.
-- No passwords, credentials, OpenIDs, user contacts or row contents are selected.
SELECT DATABASE() AS selected_database, VERSION() AS mysql_version;

SELECT TABLE_NAME,ENGINE,TABLE_COLLATION
FROM information_schema.TABLES
WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN
('user','school','admin_user','admin_school_relation','status_notification','user_status_message',
 'msg_template_config','wx_subscribe_delivery','commercial_order','campus_cooperation',
 'campus_cooperation_application','campus_cooperation_image')
ORDER BY TABLE_NAME;

SELECT TABLE_NAME,COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,GENERATION_EXPRESSION
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA=DATABASE() AND
  (TABLE_NAME IN ('commercial_order','campus_cooperation','campus_cooperation_application','campus_cooperation_image',
                 'status_notification','user_status_message','msg_template_config','wx_subscribe_delivery','admin_school_relation')
   OR (TABLE_NAME IN ('user','school','admin_user') AND COLUMN_NAME IN ('id','school_id','role','status')))
ORDER BY TABLE_NAME,ORDINAL_POSITION;

SELECT TABLE_NAME,INDEX_NAME,NON_UNIQUE,SEQ_IN_INDEX,COLUMN_NAME
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN
('commercial_order','campus_cooperation','campus_cooperation_application','campus_cooperation_image',
 'status_notification','user_status_message','admin_school_relation','msg_template_config')
ORDER BY TABLE_NAME,INDEX_NAME,SEQ_IN_INDEX;

SELECT TABLE_NAME,CONSTRAINT_NAME,COLUMN_NAME,REFERENCED_TABLE_NAME,REFERENCED_COLUMN_NAME
FROM information_schema.KEY_COLUMN_USAGE
WHERE CONSTRAINT_SCHEMA=DATABASE() AND TABLE_NAME IN
('commercial_order','campus_cooperation','campus_cooperation_application','campus_cooperation_image',
 'status_notification','admin_school_relation')
ORDER BY TABLE_NAME,CONSTRAINT_NAME,ORDINAL_POSITION;

-- This independent check should yield 0 rows. It checks enabled current owners only.
SELECT rel.school_id,COUNT(*) AS enabled_owner_count
FROM admin_school_relation rel JOIN admin_user au ON au.id=rel.admin_user_id
WHERE au.role=2 AND au.status=1 AND rel.is_owner=1
GROUP BY rel.school_id HAVING COUNT(*)>1;
