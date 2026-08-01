-- 一次性对账：修复上线前遗留、没有 delivery intent 的已支付 pending 订单。
-- MySQL 5.7+。执行前请备份数据库；脚本只处理 10 分钟前且仍为 pending 的未退款订单。

DROP TEMPORARY TABLE IF EXISTS legacy_pending_delivery_reconcile;
CREATE TEMPORARY TABLE legacy_pending_delivery_reconcile (
  order_id INT PRIMARY KEY,
  has_completed_delivery TINYINT(1) NOT NULL
);

INSERT INTO legacy_pending_delivery_reconcile (order_id, has_completed_delivery)
SELECT
  o.id,
  CASE WHEN
    EXISTS (
      SELECT 1
      FROM olive_branch_sms_notice notice
      WHERE notice.order_id = o.id AND notice.status = 2
    )
    OR EXISTS (
      SELECT 1
      FROM email_promotion promotion
      WHERE promotion.order_id = o.id AND promotion.status = 2
    )
    OR EXISTS (
      SELECT 1
      FROM email_promotion promotion
      WHERE promotion.order_id = o.id
        AND promotion.max_recipients > 0
        AND promotion.total_sent >= promotion.max_recipients
    )
    OR EXISTS (
      SELECT 1
      FROM email_promotion promotion
      JOIN email_task task ON task.promotion_id = promotion.id AND task.status = 2
      WHERE promotion.order_id = o.id
      GROUP BY promotion.id, promotion.max_recipients
      HAVING COUNT(*) >= promotion.max_recipients
    )
  THEN 1 ELSE 0 END
FROM `order` o
WHERE o.status = 1
  AND o.refund_status = 0
  AND o.push_status = 'pending'
  AND o.delivery_scene IS NULL
  AND o.delivery_payload IS NULL
  AND o.updated_at < NOW() - INTERVAL 10 MINUTE;

-- 执行更新前的冻结清单；请保存该结果作为回执。
SELECT order_id, has_completed_delivery
FROM legacy_pending_delivery_reconcile
ORDER BY order_id;

START TRANSACTION;

UPDATE `order` o
JOIN legacy_pending_delivery_reconcile r ON r.order_id = o.id
SET o.push_status = 'success',
    o.push_error_message = NULL,
    o.last_push_time = NOW(),
    o.updated_at = NOW()
WHERE r.has_completed_delivery = 1
  AND o.status = 1
  AND o.refund_status = 0
  AND o.push_status = 'pending'
  AND o.delivery_scene IS NULL
  AND o.delivery_payload IS NULL;

UPDATE `order` o
JOIN legacy_pending_delivery_reconcile r ON r.order_id = o.id
SET o.push_status = 'failed',
    o.push_error_message = '历史订单未发现完成投递证据，请人工核验后显式重试或退款',
    o.last_push_time = NOW(),
    o.updated_at = NOW()
WHERE r.has_completed_delivery = 0
  AND o.status = 1
  AND o.refund_status = 0
  AND o.push_status = 'pending'
  AND o.delivery_scene IS NULL
  AND o.delivery_payload IS NULL;

COMMIT;

-- 验证应返回 0；同时输出本次订单最终状态。
SELECT COUNT(*) AS remaining_stale_legacy_pending_orders
FROM `order`
WHERE status = 1
  AND refund_status = 0
  AND push_status = 'pending'
  AND delivery_scene IS NULL
  AND delivery_payload IS NULL
  AND updated_at < NOW() - INTERVAL 10 MINUTE;

SELECT o.id, o.push_status, o.push_retry_count, o.push_error_message, o.last_push_time
FROM `order` o
JOIN legacy_pending_delivery_reconcile r ON r.order_id = o.id
ORDER BY o.id;

DROP TEMPORARY TABLE IF EXISTS legacy_pending_delivery_reconcile;
