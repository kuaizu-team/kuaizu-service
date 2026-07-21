-- MySQL 5.7+. Adds order-level push delivery state without replacing task-level audit tables.
DROP PROCEDURE IF EXISTS migrate_order_push_status;
DELIMITER $$
CREATE PROCEDURE migrate_order_push_status()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'order' AND COLUMN_NAME = 'push_status'
  ) THEN
    ALTER TABLE `order`
      ADD COLUMN push_status VARCHAR(16) NULL COMMENT 'pending/success/failed' AFTER status;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'order' AND COLUMN_NAME = 'push_retry_count'
  ) THEN
    ALTER TABLE `order`
      ADD COLUMN push_retry_count INT NOT NULL DEFAULT 0 COMMENT 'manual push retry count' AFTER push_status;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'order' AND COLUMN_NAME = 'last_push_time'
  ) THEN
    ALTER TABLE `order`
      ADD COLUMN last_push_time TIMESTAMP NULL DEFAULT NULL COMMENT 'last push attempt time' AFTER push_retry_count;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'order' AND COLUMN_NAME = 'push_error_message'
  ) THEN
    ALTER TABLE `order`
      ADD COLUMN push_error_message VARCHAR(500) NULL COMMENT 'last push failure reason' AFTER last_push_time;
  END IF;
END$$
DELIMITER ;
CALL migrate_order_push_status();
DROP PROCEDURE migrate_order_push_status;

-- Backfill the most recent delivery result for historical paid-message orders.
UPDATE `order` o
JOIN (
  SELECT ep.order_id,
         CASE ep.status WHEN 2 THEN 'success' WHEN 3 THEN 'failed' ELSE 'pending' END AS push_status,
         COALESCE(ep.completed_at, ep.started_at, ep.created_at) AS last_push_time,
         ep.error_message
  FROM email_promotion ep
  JOIN (
    SELECT order_id, MAX(id) AS id
    FROM email_promotion
    WHERE order_id IS NOT NULL
    GROUP BY order_id
  ) latest ON latest.id = ep.id
) source ON source.order_id = o.id
SET o.push_status = source.push_status,
    o.last_push_time = source.last_push_time,
    o.push_error_message = CASE WHEN source.push_status = 'failed' THEN source.error_message ELSE NULL END
WHERE o.push_status IS NULL;

UPDATE `order` o
JOIN (
  SELECT notice.order_id,
         CASE notice.status WHEN 2 THEN 'success' WHEN 3 THEN 'failed' ELSE 'pending' END AS push_status,
         COALESCE(notice.completed_at, notice.started_at, notice.created_at) AS last_push_time,
         notice.error_message
  FROM olive_branch_sms_notice notice
  JOIN (
    SELECT order_id, MAX(id) AS id
    FROM olive_branch_sms_notice
    WHERE order_id IS NOT NULL
    GROUP BY order_id
  ) latest ON latest.id = notice.id
) source ON source.order_id = o.id
SET o.push_status = source.push_status,
    o.last_push_time = source.last_push_time,
    o.push_error_message = CASE WHEN source.push_status = 'failed' THEN source.error_message ELSE NULL END
WHERE o.push_status IS NULL;
