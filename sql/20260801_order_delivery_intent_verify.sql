-- 只读验证：执行 20260801_order_delivery_intent.sql 后运行，并回传结果。

SELECT column_name, column_type, is_nullable
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'order'
  AND column_name IN ('delivery_scene', 'delivery_payload')
ORDER BY ordinal_position;

SELECT index_name, seq_in_index, column_name
FROM information_schema.statistics
WHERE table_schema = DATABASE()
  AND table_name = 'order'
  AND index_name = 'idx_order_delivery_recovery'
ORDER BY seq_in_index;

SELECT COUNT(*) AS invalid_existing_delivery_rows
FROM `order`
WHERE (delivery_scene IS NULL) <> (delivery_payload IS NULL)
   OR (delivery_scene IS NOT NULL AND delivery_scene NOT IN ('email_promotion', 'sms_notice'));
