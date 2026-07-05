ALTER TABLE `project`
  ADD COLUMN `admin_note` TEXT NULL COMMENT '管理员跟进备注' AFTER `deleted_at`,
  ADD COLUMN `admin_note_updated_at` TIMESTAMP NULL DEFAULT NULL COMMENT '管理员备注更新时间' AFTER `admin_note`;
