-- Review and apply manually before enabling AUTO_URGE_SMS_ENABLED.
-- This script is provided only; the application never creates this table.
-- Automatic state is independent of admin_sms_send_record and manual sends.
CREATE TABLE IF NOT EXISTS auto_urge_sms_state (
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
  KEY idx_auto_urge_retry (state, next_retry_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='One successful automatic URGE_PROCESS SMS per user; uncertain attempts never auto-retry';
