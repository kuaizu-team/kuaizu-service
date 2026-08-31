-- Paid message delivery compensation after deploying the 2026-08-27 fixes.
-- MySQL 5.7+. Review the preflight result before committing the guarded update.

START TRANSACTION;

-- Expected row: order 134 is paid, sms_notice, failed, retry_count=3.
SELECT id, user_id, status, refund_status, push_status, push_retry_count,
       push_error_message, delivery_scene, delivery_payload, wx_pay_no, pay_time
FROM `order`
WHERE id = 134
FOR UPDATE;

-- Restore the bounded retry allowance without creating a new order or charging again.
UPDATE `order`
SET push_status = 'failed',
    push_retry_count = 0,
    push_error_message = '待短信兼容修复部署后重新发送',
    updated_at = NOW()
WHERE id = 134
  AND status = 1
  AND refund_status = 0
  AND delivery_scene = 'sms_notice'
  AND push_status = 'failed'
  AND push_retry_count = 3
  AND wx_pay_no = '4500000384202608212855357924';

SELECT ROW_COUNT() AS compensated_sms_order_count;

COMMIT;

-- After deployment, call POST /orders/134/push/retry once as the order owner.
-- The existing failed notice/task is reused; do not create or pay another order.

-- Audit only: these email-promotion orders were cancelled and have no accepted
-- payment evidence in the database export. Reconcile them against WeChat first.
SELECT id, user_id, product_id, quantity, actual_paid, status, push_status,
       delivery_scene, delivery_payload, wx_pay_no, pay_time, created_at, updated_at
FROM `order`
WHERE id IN (131, 132, 135)
ORDER BY id;

-- Do not update a cancelled email order to paid from this script. Prefer replaying
-- its verified xpay_goods_deliver_notify callback after WeChat-side reconciliation.
