-- READ ONLY. Run after migration in EACH affected database and capture ALL result sets.
SELECT 'V01_ENV' AS receipt,DATABASE() AS database_name,VERSION() AS mysql_version,@@session.time_zone AS session_time_zone;
SELECT 'V02_COLUMNS' AS receipt,table_name,column_name,column_type,is_nullable,column_default,character_set_name,collation_name
FROM information_schema.columns WHERE table_schema=DATABASE() AND
 ((table_name='admin_sms_send_record' AND column_name='request_key') OR
 (table_name='auto_urge_sms_state' AND column_name IN ('cycle_id','closed_at','request_key','dispatched_at')))
ORDER BY table_name,ordinal_position;
SELECT 'V03_CHECKS' AS receipt,checks.* FROM (
SELECT 'SMS_REQUEST_KEY' AS check_item,CASE
 WHEN NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='admin_sms_send_record') THEN 'NOT_PRESENT'
 WHEN EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='admin_sms_send_record' AND column_name='request_key' AND data_type='varchar' AND character_maximum_length>=128 AND is_nullable='YES' AND column_default IS NULL AND extra='' AND character_set_name='utf8mb4' AND collation_name='utf8mb4_bin') THEN 'PASS' ELSE 'FAIL' END AS result
UNION ALL
SELECT 'AUTO_CYCLE_ID' AS check_item,CASE
 WHEN NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state') THEN 'NOT_PRESENT'
 WHEN EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state' AND column_name='cycle_id' AND data_type='bigint' AND is_nullable='NO' AND column_default='1' AND extra='') THEN 'PASS' ELSE 'FAIL' END AS result
UNION ALL
SELECT 'AUTO_CLOSED_AT' AS check_item,CASE
 WHEN NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state') THEN 'NOT_PRESENT'
 WHEN EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state' AND column_name='closed_at' AND data_type='datetime' AND is_nullable='YES' AND column_default IS NULL AND extra='') THEN 'PASS' ELSE 'FAIL' END AS result
UNION ALL
SELECT 'AUTO_REQUEST_KEY' AS check_item,CASE
 WHEN NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state') THEN 'NOT_PRESENT'
 WHEN EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state' AND column_name='request_key' AND data_type='varchar' AND character_maximum_length>=128 AND is_nullable='YES' AND column_default IS NULL AND extra='' AND character_set_name='utf8mb4' AND collation_name='utf8mb4_bin') THEN 'PASS' ELSE 'FAIL' END AS result
UNION ALL
SELECT 'AUTO_DISPATCHED_AT' AS check_item,CASE
 WHEN NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state') THEN 'NOT_PRESENT'
 WHEN EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state' AND column_name='dispatched_at' AND data_type='datetime' AND is_nullable='YES' AND column_default IS NULL AND extra='') THEN 'PASS' ELSE 'FAIL' END AS result
UNION ALL
SELECT 'SMS_REQUEST_KEY_UNIQUE' AS check_item,CASE
 WHEN NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='admin_sms_send_record') THEN 'NOT_PRESENT'
 WHEN EXISTS(SELECT 1 FROM (SELECT index_name FROM information_schema.statistics
 WHERE table_schema=DATABASE() AND table_name='admin_sms_send_record'
 GROUP BY index_name HAVING MAX(non_unique)=0 AND COUNT(*)=1
 AND MAX(column_name)='request_key' AND MAX(sub_part) IS NULL) correct_index) THEN 'PASS' ELSE 'FAIL' END AS result
UNION ALL
SELECT 'AUTO_USER_PRIMARY' AS check_item,CASE
 WHEN NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state') THEN 'NOT_PRESENT'
 WHEN (SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state' AND index_name='PRIMARY')=1 AND EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state' AND index_name='PRIMARY' AND column_name='user_id' AND sub_part IS NULL) THEN 'PASS' ELSE 'FAIL' END AS result
UNION ALL
SELECT 'AUTO_BASE_COLUMNS' AS check_item,CASE
 WHEN NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state') THEN 'NOT_PRESENT'
 WHEN (SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state' AND column_name IN ('user_id','state','claim_token','attempt_count','pending_count','oldest_pending_at','last_attempt_at','next_retry_at','sent_at','message_record_id','error_code','error_message','created_at','updated_at'))=14 THEN 'PASS' ELSE 'FAIL' END AS result
UNION ALL
SELECT 'SMS_ENGINE' AS check_item,CASE
 WHEN NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='admin_sms_send_record') THEN 'NOT_PRESENT'
 WHEN EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='admin_sms_send_record' AND engine='InnoDB') THEN 'PASS' ELSE 'FAIL' END AS result
UNION ALL
SELECT 'AUTO_ENGINE' AS check_item,CASE
 WHEN NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state') THEN 'NOT_PRESENT'
 WHEN EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state' AND engine='InnoDB') THEN 'PASS' ELSE 'FAIL' END AS result
) checks;
SELECT 'V04_INDEXES' AS receipt,table_name,index_name,non_unique,seq_in_index,column_name,sub_part
FROM information_schema.statistics WHERE table_schema=DATABASE()
AND table_name IN ('admin_sms_send_record','auto_urge_sms_state') ORDER BY table_name,index_name,seq_in_index;
SET @kz_audit_sql=IF(EXISTS(SELECT 1 FROM information_schema.columns
 WHERE table_schema=DATABASE() AND table_name='admin_sms_send_record' AND column_name='status'),
 'SELECT ''V05_SMS_COUNTS'' AS receipt,status,COUNT(*) AS records FROM admin_sms_send_record GROUP BY status','SELECT ''V05_SMS_COUNTS'' AS receipt,''TABLE_OR_COLUMN_NOT_PRESENT'' AS result');
PREPARE kz_audit_stmt FROM @kz_audit_sql;
EXECUTE kz_audit_stmt;
DEALLOCATE PREPARE kz_audit_stmt;
SET @kz_audit_sql=IF(EXISTS(SELECT 1 FROM information_schema.columns
 WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state' AND column_name='state'),
 'SELECT ''V06_AUTO_COUNTS'' AS receipt,state,COUNT(*) AS records FROM auto_urge_sms_state GROUP BY state','SELECT ''V06_AUTO_COUNTS'' AS receipt,''TABLE_OR_COLUMN_NOT_PRESENT'' AS result');
PREPARE kz_audit_stmt FROM @kz_audit_sql;
EXECUTE kz_audit_stmt;
DEALLOCATE PREPARE kz_audit_stmt;
SET @kz_audit_sql=IF(EXISTS(SELECT 1 FROM information_schema.columns
 WHERE table_schema=DATABASE() AND table_name='admin_sms_send_record' AND column_name='request_key'),
 'SELECT ''V07_DUPLICATE_KEYS'' AS receipt,COUNT(*) AS duplicate_key_groups FROM (SELECT request_key FROM admin_sms_send_record WHERE request_key IS NOT NULL GROUP BY request_key HAVING COUNT(*)>1) d','SELECT ''V07_DUPLICATE_KEYS'' AS receipt,''TABLE_OR_COLUMN_NOT_PRESENT'' AS result');
PREPARE kz_audit_stmt FROM @kz_audit_sql;
EXECUTE kz_audit_stmt;
DEALLOCATE PREPARE kz_audit_stmt;
