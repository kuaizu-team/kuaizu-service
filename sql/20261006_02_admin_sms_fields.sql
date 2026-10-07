-- MESSAGE-CENTER DB. Backup first and stop related SMS writers.
-- Run the ENTIRE file. Requires ALTER and CREATE ROUTINE permissions.
-- On ANY error stop and return its text. DDL implicitly commits.
-- Preserves all historical records and sending statuses. Does not send messages.
DELIMITER $$
DROP PROCEDURE IF EXISTS kz_audit_20261006_admin_sms_fields$$
CREATE PROCEDURE kz_audit_20261006_admin_sms_fields()
BEGIN
 DECLARE v_count INT DEFAULT 0;
 IF DATABASE() IS NULL THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Select the message-center database first';
 END IF;
 SELECT COUNT(*) INTO v_count FROM information_schema.tables WHERE table_schema=DATABASE()
 AND table_name='admin_sms_send_record' AND engine='InnoDB';
 IF v_count<>1 THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='SMS table missing or not InnoDB; return preflight';
 END IF;
 SELECT COUNT(*) INTO v_count FROM information_schema.columns WHERE table_schema=DATABASE()
 AND table_name='admin_sms_send_record' AND column_name IN ('id','user_id','template_key','status','error_code');
 IF v_count<>5 THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='SMS base columns differ; return preflight';
 END IF;
 IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE()
 AND table_name='admin_sms_send_record' AND column_name='request_key') THEN
  IF NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE()
  AND table_name='admin_sms_send_record' AND column_name='request_key' AND data_type='varchar' AND character_maximum_length>=128 AND is_nullable='YES' AND column_default IS NULL AND extra='' AND character_set_name='utf8mb4' AND collation_name='utf8mb4_bin') THEN
   SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Existing SMS request_key differs; do not alter or clear data';
  END IF;
 ELSE
  ALTER TABLE admin_sms_send_record ADD COLUMN request_key
   VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL
   COMMENT 'Stable request key; NULL preserves legacy/manual SMS';
 END IF;
 SELECT COUNT(*) INTO v_count FROM (SELECT request_key FROM admin_sms_send_record
 WHERE request_key IS NOT NULL GROUP BY request_key HAVING COUNT(*)>1) d;
 IF v_count<>0 THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Duplicate SMS request keys; return results without deleting records';
 END IF;
 SELECT COUNT(*) INTO v_count FROM (SELECT index_name FROM information_schema.statistics
 WHERE table_schema=DATABASE() AND table_name='admin_sms_send_record'
 GROUP BY index_name HAVING MAX(non_unique)=0 AND COUNT(*)=1
 AND MAX(column_name)='request_key' AND MAX(sub_part) IS NULL) correct_indexes;
 IF v_count=0 THEN
  IF EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE()
  AND table_name='admin_sms_send_record' AND index_name='uk_admin_sms_request_key') THEN
   SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Named SMS index differs; return SHOW INDEX without dropping it';
  END IF;
  ALTER TABLE admin_sms_send_record ADD UNIQUE KEY uk_admin_sms_request_key (request_key);
 END IF;
END$$
CALL kz_audit_20261006_admin_sms_fields()$$
DROP PROCEDURE kz_audit_20261006_admin_sms_fields$$
DELIMITER ;
SELECT 'M02_COMPLETE' AS receipt,DATABASE() AS database_name;
