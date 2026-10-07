-- BUSINESS DB. Backup first, AUTO_URGE_SMS_ENABLED=false, stop backend writers.
-- Run the ENTIRE file. Requires ALTER, CREATE TABLE and CREATE ROUTINE permissions.
-- On ANY error stop and return its text. DDL implicitly commits.
-- No historical sent/unknown state is reset and no pending business record is changed.
DELIMITER $$
DROP PROCEDURE IF EXISTS kz_audit_20261006_auto_urge_fields$$
CREATE PROCEDURE kz_audit_20261006_auto_urge_fields()
BEGIN
 DECLARE v_count INT DEFAULT 0;
 IF DATABASE() IS NULL THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Select the business database first';
 END IF;
 SELECT COUNT(*) INTO v_count FROM information_schema.tables WHERE table_schema=DATABASE()
 AND table_name IN ('user','project','project_members','project_application','olive_branch_record') AND engine='InnoDB';
 IF v_count<>5 THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Required business tables missing or not InnoDB; return preflight';
 END IF;
 IF EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='auto_urge_sms_state') THEN
  IF NOT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE()
  AND table_name='auto_urge_sms_state' AND engine='InnoDB') THEN
   SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='auto_urge_sms_state is not InnoDB; return preflight';
  END IF;
  SELECT COUNT(*) INTO v_count FROM information_schema.columns WHERE table_schema=DATABASE()
  AND table_name='auto_urge_sms_state' AND column_name IN ('user_id','state','claim_token','attempt_count','pending_count','oldest_pending_at','last_attempt_at','next_retry_at','sent_at','message_record_id','error_code','error_message','created_at','updated_at');
  IF v_count<>14 THEN
   SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Existing auto-urge base columns differ; return preflight';
  END IF;
  SELECT COUNT(*) INTO v_count FROM information_schema.statistics WHERE table_schema=DATABASE()
  AND table_name='auto_urge_sms_state' AND index_name='PRIMARY';
  IF v_count<>1 OR NOT EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE()
  AND table_name='auto_urge_sms_state' AND index_name='PRIMARY' AND column_name='user_id' AND sub_part IS NULL) THEN
   SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Existing auto-urge primary key differs; return preflight';
  END IF;
  IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE()
  AND table_name='auto_urge_sms_state' AND column_name='cycle_id') AND
  NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE()
  AND table_name='auto_urge_sms_state' AND column_name='cycle_id' AND data_type='bigint' AND is_nullable='NO' AND column_default='1' AND extra='') THEN
   SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Existing auto-urge cycle_id differs; return preflight without changing it';
  END IF;
  IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE()
  AND table_name='auto_urge_sms_state' AND column_name='closed_at') AND
  NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE()
  AND table_name='auto_urge_sms_state' AND column_name='closed_at' AND data_type='datetime' AND is_nullable='YES' AND column_default IS NULL AND extra='') THEN
   SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Existing auto-urge closed_at differs; return preflight without changing it';
  END IF;
  IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE()
  AND table_name='auto_urge_sms_state' AND column_name='request_key') AND
  NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE()
  AND table_name='auto_urge_sms_state' AND column_name='request_key' AND data_type='varchar' AND character_maximum_length>=128 AND is_nullable='YES' AND column_default IS NULL AND extra='' AND character_set_name='utf8mb4' AND collation_name='utf8mb4_bin') THEN
   SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Existing auto-urge request_key differs; return preflight without changing it';
  END IF;
  IF EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE()
  AND table_name='auto_urge_sms_state' AND column_name='dispatched_at') AND
  NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE()
  AND table_name='auto_urge_sms_state' AND column_name='dispatched_at' AND data_type='datetime' AND is_nullable='YES' AND column_default IS NULL AND extra='') THEN
   SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Existing auto-urge dispatched_at differs; return preflight without changing it';
  END IF;
 ELSE
  CREATE TABLE auto_urge_sms_state (
   user_id INT NOT NULL,
   state VARCHAR(16) NOT NULL DEFAULT 'ready' COMMENT 'ready/claimed/sent/failed/unknown',
   claim_token VARCHAR(36) NULL,
   attempt_count INT NOT NULL DEFAULT 0,
   pending_count INT NULL,
   oldest_pending_at DATETIME NULL,
   last_attempt_at DATETIME NULL,
   next_retry_at DATETIME NULL,
   sent_at DATETIME NULL,
   message_record_id BIGINT NULL,
   error_code VARCHAR(128) NULL,
   error_message VARCHAR(500) NULL,
   created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
   updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
   PRIMARY KEY (user_id),
   KEY idx_auto_urge_retry (state,next_retry_at)
  ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Automatic SMS state; preserve history during cycle migration';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE()
 AND table_name='auto_urge_sms_state' AND column_name='cycle_id') THEN
  ALTER TABLE auto_urge_sms_state ADD COLUMN cycle_id BIGINT NOT NULL DEFAULT 1 COMMENT 'Historical state remains in cycle 1';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE()
 AND table_name='auto_urge_sms_state' AND column_name='closed_at') THEN
  ALTER TABLE auto_urge_sms_state ADD COLUMN closed_at DATETIME NULL COMMENT 'Committed observation of a cleared cycle';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE()
 AND table_name='auto_urge_sms_state' AND column_name='request_key') THEN
  ALTER TABLE auto_urge_sms_state ADD COLUMN request_key VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL COMMENT 'Stable SMS key for the current cycle';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE()
 AND table_name='auto_urge_sms_state' AND column_name='dispatched_at') THEN
  ALTER TABLE auto_urge_sms_state ADD COLUMN dispatched_at DATETIME NULL COMMENT 'Persisted dispatch boundary for reconciliation';
 END IF;
END$$
CALL kz_audit_20261006_auto_urge_fields()$$
DROP PROCEDURE kz_audit_20261006_auto_urge_fields$$
DELIMITER ;
SELECT 'M03_COMPLETE' AS receipt,DATABASE() AS database_name,
 'Keep AUTO_URGE_SMS_ENABLED=false until code integration and verification' AS next_step;
