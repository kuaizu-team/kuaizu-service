-- Email promotion batches migration for MySQL 5.7+.
-- Safe to re-run. Keeps project promotion batch metadata and recipient snapshots
-- aligned with the current production table shape.

DROP PROCEDURE IF EXISTS _email_promotion_batches_add_columns;
DELIMITER $$
CREATE PROCEDURE _email_promotion_batches_add_columns()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'email_promotion'
      AND column_name = 'channel'
  ) THEN
    ALTER TABLE email_promotion
      ADD COLUMN channel VARCHAR(32) DEFAULT NULL COMMENT 'Promotion channel' AFTER id;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'email_promotion'
      AND column_name = 'business_tag'
  ) THEN
    ALTER TABLE email_promotion
      ADD COLUMN business_tag VARCHAR(64) DEFAULT NULL COMMENT 'Business tag' AFTER channel;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'email_promotion'
      AND column_name = 'trace_id'
  ) THEN
    ALTER TABLE email_promotion
      ADD COLUMN trace_id VARCHAR(128) DEFAULT NULL COMMENT 'Business trace ID' AFTER business_tag;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'email_promotion'
      AND column_name = 'strategy'
  ) THEN
    ALTER TABLE email_promotion
      ADD COLUMN strategy VARCHAR(32) NOT NULL DEFAULT 'region' COMMENT 'Recipient selection strategy' AFTER creator_id;
  END IF;
END$$
DELIMITER ;
CALL _email_promotion_batches_add_columns();
DROP PROCEDURE IF EXISTS _email_promotion_batches_add_columns;

CREATE TABLE IF NOT EXISTS email_promotion_recipient (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  promotion_id INT NOT NULL COMMENT 'Promotion batch ID',
  email_task_id BIGINT DEFAULT NULL COMMENT 'Related email task ID',
  project_id INT NOT NULL COMMENT 'Project ID',
  user_id INT NOT NULL COMMENT 'Recipient user ID',
  status TINYINT NOT NULL DEFAULT 0 COMMENT '0 pending, 1 sending, 2 success, 3 failed',
  sent_at TIMESTAMP NULL DEFAULT NULL COMMENT 'Sent time',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'Created time',
  PRIMARY KEY (id),
  UNIQUE KEY uk_epr_promotion_user (promotion_id, user_id),
  KEY idx_epr_project_user_created (project_id, user_id, created_at),
  KEY idx_epr_promotion_created (promotion_id, created_at),
  KEY idx_epr_email_task (email_task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Email promotion recipient snapshot';

DROP PROCEDURE IF EXISTS _email_promotion_batches_create_indexes;
DELIMITER $$
CREATE PROCEDURE _email_promotion_batches_create_indexes()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'email_promotion'
      AND index_name = 'idx_ep_project_order'
  ) THEN
    CREATE INDEX idx_ep_project_order ON email_promotion (project_id, order_id);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'email_promotion'
      AND index_name = 'idx_ep_business_trace'
  ) THEN
    CREATE INDEX idx_ep_business_trace ON email_promotion (channel, business_tag, trace_id);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'email_promotion_recipient'
      AND index_name = 'uk_epr_promotion_user'
  ) THEN
    CREATE UNIQUE INDEX uk_epr_promotion_user ON email_promotion_recipient (promotion_id, user_id);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'email_promotion_recipient'
      AND index_name = 'idx_epr_project_user_created'
  ) THEN
    CREATE INDEX idx_epr_project_user_created ON email_promotion_recipient (project_id, user_id, created_at);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'email_promotion_recipient'
      AND index_name = 'idx_epr_promotion_created'
  ) THEN
    CREATE INDEX idx_epr_promotion_created ON email_promotion_recipient (promotion_id, created_at);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'email_promotion_recipient'
      AND index_name = 'idx_epr_email_task'
  ) THEN
    CREATE INDEX idx_epr_email_task ON email_promotion_recipient (email_task_id);
  END IF;
END$$
DELIMITER ;
CALL _email_promotion_batches_create_indexes();
DROP PROCEDURE IF EXISTS _email_promotion_batches_create_indexes;

