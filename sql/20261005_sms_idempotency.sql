-- Apply manually, with AUTO_URGE_SMS_ENABLED=false and message center stopped.
-- Run migration_auto_urge_sms_state.sql first if the table does not exist.
-- Existing successful/unknown records are preserved. No historical SMS is resent.
SET @audit_ddl = IF(
 (SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='admin_sms_send_record' AND column_name='request_key')=0,
 'ALTER TABLE admin_sms_send_record ADD COLUMN request_key VARCHAR(128) NULL', 'SELECT 1 AS already_applied');
PREPARE audit_stmt FROM @audit_ddl; EXECUTE audit_stmt; DEALLOCATE PREPARE audit_stmt;
SET @audit_ddl = IF(
 (SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='admin_sms_send_record' AND index_name='uk_admin_sms_request_key')=0,
 'ALTER TABLE admin_sms_send_record ADD UNIQUE KEY uk_admin_sms_request_key (request_key)', 'SELECT 1 AS already_applied');
PREPARE audit_stmt FROM @audit_ddl; EXECUTE audit_stmt; DEALLOCATE PREPARE audit_stmt;
SET @audit_ddl = IF(
 (SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state' AND column_name='cycle_id')=0,
 'ALTER TABLE auto_urge_sms_state ADD COLUMN cycle_id BIGINT NOT NULL DEFAULT 1', 'SELECT 1 AS already_applied');
PREPARE audit_stmt FROM @audit_ddl; EXECUTE audit_stmt; DEALLOCATE PREPARE audit_stmt;
SET @audit_ddl = IF(
 (SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state' AND column_name='closed_at')=0,
 'ALTER TABLE auto_urge_sms_state ADD COLUMN closed_at DATETIME NULL', 'SELECT 1 AS already_applied');
PREPARE audit_stmt FROM @audit_ddl; EXECUTE audit_stmt; DEALLOCATE PREPARE audit_stmt;
SET @audit_ddl = IF(
 (SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state' AND column_name='request_key')=0,
 'ALTER TABLE auto_urge_sms_state ADD COLUMN request_key VARCHAR(128) NULL', 'SELECT 1 AS already_applied');
PREPARE audit_stmt FROM @audit_ddl; EXECUTE audit_stmt; DEALLOCATE PREPARE audit_stmt;
SET @audit_ddl = IF(
 (SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state' AND column_name='dispatched_at')=0,
 'ALTER TABLE auto_urge_sms_state ADD COLUMN dispatched_at DATETIME NULL', 'SELECT 1 AS already_applied');
PREPARE audit_stmt FROM @audit_ddl; EXECUTE audit_stmt; DEALLOCATE PREPARE audit_stmt;
SELECT table_name,column_name,column_type,is_nullable,column_default FROM information_schema.columns
WHERE table_schema=DATABASE() AND ((table_name='admin_sms_send_record' AND column_name='request_key')
 OR (table_name='auto_urge_sms_state' AND column_name IN ('cycle_id','closed_at','request_key','dispatched_at')));
SHOW INDEX FROM admin_sms_send_record;
