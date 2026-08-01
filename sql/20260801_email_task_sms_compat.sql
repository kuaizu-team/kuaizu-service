-- 允许 email_task 同时承载邮件与短信任务。
-- 幂等执行：字段已经可空时不会修改表结构。

DROP PROCEDURE IF EXISTS migrate_email_task_sms_compat;
DELIMITER $$
CREATE PROCEDURE migrate_email_task_sms_compat()
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'email_task'
      AND column_name = 'promotion_id'
      AND is_nullable = 'NO'
  ) THEN
    ALTER TABLE email_task
      MODIFY COLUMN promotion_id int(11) NULL COMMENT '邮件推广记录ID；短信任务为空';
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'email_task'
      AND column_name = 'recipient_email'
      AND is_nullable = 'NO'
  ) THEN
    ALTER TABLE email_task
      MODIFY COLUMN recipient_email varchar(255) NULL COMMENT '收件人邮箱；短信任务为空';
  END IF;
END$$
DELIMITER ;

CALL migrate_email_task_sms_compat();
DROP PROCEDURE IF EXISTS migrate_email_task_sms_compat;
