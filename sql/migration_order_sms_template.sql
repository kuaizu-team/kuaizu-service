-- MySQL 5.7: record the SMS business template used by service orders.
DROP PROCEDURE IF EXISTS migrate_order_sms_template;
DELIMITER $$
CREATE PROCEDURE migrate_order_sms_template()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'order' AND COLUMN_NAME = 'template_code'
  ) THEN
    ALTER TABLE `order`
      ADD COLUMN `template_code` VARCHAR(64) NULL COMMENT 'SMS template code' AFTER `product_id`;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'order' AND COLUMN_NAME = 'template_name'
  ) THEN
    ALTER TABLE `order`
      ADD COLUMN `template_name` VARCHAR(100) NULL COMMENT 'SMS template display name' AFTER `template_code`;
  END IF;
END$$
DELIMITER ;
CALL migrate_order_sms_template();
DROP PROCEDURE migrate_order_sms_template;