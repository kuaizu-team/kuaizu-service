-- Read-only verification. Run after 20260801_wechat_subscribe_delivery.sql.

SELECT COUNT(*) AS template_count,
       COUNT(DISTINCT `template_id`) AS distinct_template_id_count,
       SUM(CASE WHEN CHAR_LENGTH(COALESCE(`remark`, '')) > 20 THEN 1 ELSE 0 END) AS overlong_remark_count,
       SUM(CASE WHEN `enabled` = 1 THEN 1 ELSE 0 END) AS locally_enabled_count,
       SUM(CASE WHEN `platform_status` = 'ENABLED' THEN 1 ELSE 0 END) AS platform_enabled_count
FROM `msg_template_config`
WHERE `biz_key` LIKE 'MSG\_%';

SELECT `column_name`, `column_type`, `is_nullable`, `column_default`
FROM information_schema.columns
WHERE table_schema = DATABASE()
  AND table_name = 'msg_template_config'
  AND column_name IN ('remark', 'enabled', 'platform_status', 'platform_verified_at')
ORDER BY ordinal_position;

SELECT `table_name`, `index_name`, GROUP_CONCAT(`column_name` ORDER BY `seq_in_index`) AS indexed_columns
FROM information_schema.statistics
WHERE table_schema = DATABASE()
  AND (
    (`table_name` = 'subscribe' AND `index_name` = 'idx_subscribe_biz_status_user')
    OR (`table_name` = 'msg_template_config' AND `index_name` = 'uk_msg_template_template_id')
    OR (`table_name` = 'wx_subscribe_delivery')
    OR (`table_name` = 'wx_subscribe_status_history')
  )
GROUP BY `table_name`, `index_name`
ORDER BY `table_name`, `index_name`;

SELECT `status`, COUNT(*) AS delivery_count
FROM `wx_subscribe_delivery`
GROUP BY `status`
ORDER BY `status`;
