-- Read-only post-deployment baseline. The export can prove database state but
-- not the currently running process, Redis ownership, or RabbitMQ consumers.

SELECT id, order_id, channel, business_tag, status, total_sent,
       processing_epoch, processing_token, completed_at, error_message
FROM email_promotion
WHERE id IN (28, 31, 35, 69, 71, 76)
ORDER BY id;

SELECT
  COUNT(*) AS legacy_unfinished_count,
  SUM(status = 0) AS legacy_pending_count,
  SUM(status = 1) AS legacy_sending_count
FROM email_promotion
WHERE status IN (0, 1)
  AND (channel IS NULL OR business_tag IS NULL);

SELECT id, order_id, project_id, status, total_sent, started_at, created_at
FROM email_promotion
WHERE status IN (0, 1)
  AND (channel IS NULL OR business_tag IS NULL)
ORDER BY id;

SELECT id, order_id, project_id, status, processing_epoch,
       processing_token, started_at, created_at
FROM email_promotion
WHERE channel = 'EMAIL' AND business_tag = 'project_promotion'
  AND status IN (0, 1)
ORDER BY id;
