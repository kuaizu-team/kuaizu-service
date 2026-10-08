-- READ ONLY. Run in each actual deployment DB (once if business/message-center share a DB).
SELECT 'R01_ENV' AS receipt,DATABASE() AS database_name,VERSION() AS mysql_version,
 @@session.time_zone AS session_time_zone,@@global.time_zone AS global_time_zone;
SELECT 'R02_TABLES' AS receipt,table_name,engine FROM information_schema.tables
 WHERE table_schema=DATABASE() AND table_name IN
 ('user','project','project_members','project_application','olive_branch_record',
 'admin_sms_send_record','auto_urge_sms_state','wx_subscribe_delivery','welcome_email_delivery') ORDER BY table_name;
SELECT 'R03_COLUMNS' AS receipt,table_name,column_name,column_type,is_nullable,
 column_default,character_set_name,collation_name FROM information_schema.columns
 WHERE table_schema=DATABASE() AND table_name IN
 ('admin_sms_send_record','auto_urge_sms_state','wx_subscribe_delivery','welcome_email_delivery')
 ORDER BY table_name,ordinal_position;
SELECT 'R04_INDEXES' AS receipt,table_name,index_name,non_unique,seq_in_index,column_name,sub_part
 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name IN
 ('admin_sms_send_record','auto_urge_sms_state','wx_subscribe_delivery','welcome_email_delivery')
 ORDER BY table_name,index_name,seq_in_index;
SET @kz_audit_sql=IF(EXISTS(SELECT 1 FROM information_schema.columns
 WHERE table_schema=DATABASE() AND table_name='admin_sms_send_record' AND column_name='status'),
 'SELECT ''R05_SMS_COUNTS'' AS receipt,status,COUNT(*) AS records FROM admin_sms_send_record GROUP BY status','SELECT ''R05_SMS_COUNTS'' AS receipt,''TABLE_OR_COLUMN_NOT_PRESENT'' AS result');
PREPARE kz_audit_stmt FROM @kz_audit_sql;
EXECUTE kz_audit_stmt;
DEALLOCATE PREPARE kz_audit_stmt;
SET @kz_audit_sql=IF(EXISTS(SELECT 1 FROM information_schema.columns
 WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state' AND column_name='state'),
 'SELECT ''R06_AUTO_COUNTS'' AS receipt,state,COUNT(*) AS records FROM auto_urge_sms_state GROUP BY state','SELECT ''R06_AUTO_COUNTS'' AS receipt,''TABLE_OR_COLUMN_NOT_PRESENT'' AS result');
PREPARE kz_audit_stmt FROM @kz_audit_sql;
EXECUTE kz_audit_stmt;
DEALLOCATE PREPARE kz_audit_stmt;
SET @kz_audit_sql=IF(EXISTS(SELECT 1 FROM information_schema.columns
 WHERE table_schema=DATABASE() AND table_name='wx_subscribe_delivery' AND column_name='status'),
 'SELECT ''R07_WX_COUNTS'' AS receipt,status,COUNT(*) AS records FROM wx_subscribe_delivery GROUP BY status','SELECT ''R07_WX_COUNTS'' AS receipt,''TABLE_OR_COLUMN_NOT_PRESENT'' AS result');
PREPARE kz_audit_stmt FROM @kz_audit_sql;
EXECUTE kz_audit_stmt;
DEALLOCATE PREPARE kz_audit_stmt;
SET @kz_audit_sql=IF(EXISTS(SELECT 1 FROM information_schema.columns
 WHERE table_schema=DATABASE() AND table_name='welcome_email_delivery' AND column_name='status'),
 'SELECT ''R08_WELCOME_COUNTS'' AS receipt,status,COUNT(*) AS records FROM welcome_email_delivery GROUP BY status','SELECT ''R08_WELCOME_COUNTS'' AS receipt,''TABLE_OR_COLUMN_NOT_PRESENT'' AS result');
PREPARE kz_audit_stmt FROM @kz_audit_sql;
EXECUTE kz_audit_stmt;
DEALLOCATE PREPARE kz_audit_stmt;
SET @kz_audit_sql=IF(EXISTS(SELECT 1 FROM information_schema.columns
 WHERE table_schema=DATABASE() AND table_name='admin_sms_send_record' AND column_name='request_key'),
 'SELECT ''R09_DUPLICATE_KEYS'' AS receipt,COUNT(*) AS duplicate_key_groups FROM (SELECT request_key FROM admin_sms_send_record WHERE request_key IS NOT NULL GROUP BY request_key HAVING COUNT(*)>1) d','SELECT ''R09_DUPLICATE_KEYS'' AS receipt,''TABLE_OR_COLUMN_NOT_PRESENT'' AS result');
PREPARE kz_audit_stmt FROM @kz_audit_sql;
EXECUTE kz_audit_stmt;
DEALLOCATE PREPARE kz_audit_stmt;
