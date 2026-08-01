-- 只读验证：promotion_id、recipient_email 应为 YES，recipient_phone 应存在。
SELECT column_name, column_type, is_nullable
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'email_task'
  AND column_name IN ('promotion_id', 'recipient_email', 'recipient_phone')
ORDER BY ordinal_position;
