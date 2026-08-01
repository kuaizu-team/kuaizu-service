-- 支付前交付意图：必须先执行本迁移，再部署依赖 delivery_* 字段的后端代码。
-- 本脚本幂等；不回填历史订单，不改变现有订单状态。

DROP PROCEDURE IF EXISTS migrate_order_delivery_intent;
DELIMITER $$
CREATE PROCEDURE migrate_order_delivery_intent()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'order' AND column_name = 'delivery_scene'
  ) THEN
    ALTER TABLE `order`
      ADD COLUMN delivery_scene VARCHAR(32) NULL COMMENT 'email_promotion/sms_notice' AFTER push_error_message;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'order' AND column_name = 'delivery_payload'
  ) THEN
    ALTER TABLE `order`
      ADD COLUMN delivery_payload JSON NULL COMMENT 'immutable delivery context captured before payment' AFTER delivery_scene;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'order' AND index_name = 'idx_order_delivery_recovery'
  ) THEN
    ALTER TABLE `order`
      ADD INDEX idx_order_delivery_recovery (status, push_status, delivery_scene, updated_at);
  END IF;
END$$
DELIMITER ;

CALL migrate_order_delivery_intent();
DROP PROCEDURE IF EXISTS migrate_order_delivery_intent;
