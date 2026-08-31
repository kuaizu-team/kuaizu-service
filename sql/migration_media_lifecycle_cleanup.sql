-- Media lifecycle cleanup indexes.
-- MySQL 5.7+; safe to run repeatedly. No business data is deleted here.

SET @media_unattached_idx_exists := (
  SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'media_upload'
    AND INDEX_NAME = 'idx_media_upload_unattached'
);
SET @sql := IF(@media_unattached_idx_exists = 0,
  'ALTER TABLE media_upload ADD KEY idx_media_upload_unattached (attached_type, created_at, object_key(128))',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @media_cleanup_claim_idx_exists := (
  SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'media_upload'
    AND INDEX_NAME = 'idx_media_upload_cleanup_claim'
);
SET @sql := IF(@media_cleanup_claim_idx_exists = 0,
  'ALTER TABLE media_upload ADD KEY idx_media_upload_cleanup_claim (attached_type, attached_at, object_key(128))',
  'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Pre-deployment verification only. The service reclaims these rows after
-- deployment once they have been unattached for at least 24 hours.
SELECT media_type, COUNT(*) AS stale_unattached_count
FROM media_upload
WHERE attached_type IS NULL
  AND created_at < NOW() - INTERVAL 24 HOUR
GROUP BY media_type;
