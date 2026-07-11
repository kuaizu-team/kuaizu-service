ALTER TABLE `admin_user`
    ADD COLUMN `commission_rate` DECIMAL(5,2) NOT NULL DEFAULT 0.00 COMMENT 'commission rate percent'
    AFTER `finance_remark`;
