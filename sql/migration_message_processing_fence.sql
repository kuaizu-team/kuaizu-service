-- Required before deploying the fenced message-center worker.
-- Safe to rerun; no application executes this file automatically.

DROP PROCEDURE IF EXISTS _add_message_processing_fence;
DELIMITER $$
CREATE PROCEDURE _add_message_processing_fence()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'email_promotion'
      AND column_name = 'processing_epoch'
  ) THEN
    ALTER TABLE email_promotion
      ADD COLUMN processing_epoch INT NOT NULL DEFAULT 0
        COMMENT 'Monotonic processing attempt used for fencing'
        AFTER status;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'email_promotion'
      AND column_name = 'processing_token'
  ) THEN
    ALTER TABLE email_promotion
      ADD COLUMN processing_token VARCHAR(64) NULL
        COMMENT 'Redis ownership token for the active attempt'
        AFTER processing_epoch;
  END IF;
END$$
DELIMITER ;

CALL _add_message_processing_fence();
DROP PROCEDURE IF EXISTS _add_message_processing_fence;
