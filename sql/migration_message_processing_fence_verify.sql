-- Read-only deployment gate for migration_message_processing_fence.sql.
-- Expected result: expected_column_count=2, passed_column_count=2, verification_status=PASS.

SELECT
  2 AS expected_column_count,
  COALESCE(SUM(
    CASE
      WHEN column_name = 'processing_epoch'
        AND data_type = 'int'
        AND is_nullable = 'NO'
        AND column_default = '0'
      THEN 1
      WHEN column_name = 'processing_token'
        AND data_type = 'varchar'
        AND character_maximum_length = 64
        AND is_nullable = 'YES'
      THEN 1
      ELSE 0
    END
  ), 0) AS passed_column_count,
  CASE
    WHEN COALESCE(SUM(
      CASE
        WHEN column_name = 'processing_epoch'
          AND data_type = 'int'
          AND is_nullable = 'NO'
          AND column_default = '0'
        THEN 1
        WHEN column_name = 'processing_token'
          AND data_type = 'varchar'
          AND character_maximum_length = 64
          AND is_nullable = 'YES'
        THEN 1
        ELSE 0
      END
    ), 0) = 2 THEN 'PASS'
    ELSE 'FAIL'
  END AS verification_status
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'email_promotion'
  AND column_name IN ('processing_epoch', 'processing_token');
