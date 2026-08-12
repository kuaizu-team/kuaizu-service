-- Read-only gate for normalized contact data and unique indexes.

SELECT
  SUM(phone IS NOT NULL AND phone <> TRIM(phone)) AS phone_needs_trim,
  SUM(email IS NOT NULL AND email <> LOWER(TRIM(email))) AS email_needs_normalize,
  (SELECT COUNT(*) FROM (
    SELECT phone FROM `user` WHERE phone IS NOT NULL GROUP BY phone HAVING COUNT(*) > 1
  ) duplicate_phone) AS duplicate_phone_groups,
  (SELECT COUNT(*) FROM (
    SELECT LOWER(TRIM(email)) normalized_email FROM `user`
    WHERE email IS NOT NULL GROUP BY LOWER(TRIM(email)) HAVING COUNT(*) > 1
  ) duplicate_email) AS duplicate_email_groups
FROM `user`;

SELECT
  2 AS expected_index_count,
  COUNT(*) AS passed_index_count,
  CASE WHEN COUNT(*) = 2 THEN 'PASS' ELSE 'FAIL' END AS verification_status
FROM (
  SELECT index_name, non_unique,
         GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ',') AS indexed_columns
  FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND table_name = 'user'
    AND index_name IN ('uq_user_phone', 'uq_user_email')
  GROUP BY index_name, non_unique
) actual
WHERE non_unique = 0
  AND ((index_name = 'uq_user_phone' AND indexed_columns = 'phone')
    OR (index_name = 'uq_user_email' AND indexed_columns = 'email'));
