-- READ ONLY. Supplement the first receipt; execute in the same intended database.
-- No credentials, user contacts or business rows are selected.
SELECT DATABASE() AS selected_database, VERSION() AS mysql_version,
       @@innodb_page_size AS innodb_page_size,
       @@innodb_default_row_format AS innodb_default_row_format;

-- Required by the existing subscription framework; omitted from the first inventory.
SELECT TABLE_NAME, ENGINE, ROW_FORMAT, TABLE_COLLATION
FROM information_schema.TABLES
WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN
('wx_subscribe_status_history','wx_subscribe_delivery');

SELECT TABLE_NAME, INDEX_NAME, NON_UNIQUE, SEQ_IN_INDEX, COLUMN_NAME
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN
('wx_subscribe_status_history','wx_subscribe_delivery')
ORDER BY TABLE_NAME,INDEX_NAME,SEQ_IN_INDEX;

-- A nullable admin status is a legacy schema fact, not proof that NULL rows exist.
SELECT COUNT(*) AS admin_null_status_count
FROM admin_user WHERE status IS NULL;

-- Explicit count records the empty-result meaning lost in text-only exports.
SELECT COUNT(*) AS duplicate_enabled_school_owner_groups
FROM (
  SELECT rel.school_id
  FROM admin_school_relation rel JOIN admin_user au ON au.id=rel.admin_user_id
  WHERE au.role=2 AND au.status=1 AND rel.is_owner=1
  GROUP BY rel.school_id HAVING COUNT(*)>1
) AS duplicated_school_owners;

-- Public template metadata only. Empty result means this key/ID is not configured yet.
SELECT biz_key,template_id,content_json,enabled
FROM msg_template_config
WHERE biz_key='MSG_CAMPUS_COOPERATION_RESULT'
   OR template_id='PiZRtGMWnm2e5oi9KGqjtNE78ORAXJLFf4gJk_TJeEA';
