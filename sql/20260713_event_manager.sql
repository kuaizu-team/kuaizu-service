-- 赛事管理员角色与赛事一对一关联（MySQL 8+）
-- admin_user.role 采用整数，不需要 ALTER ENUM；4 = EVENT_MANAGER。
ALTER TABLE event
  ADD COLUMN admin_id INT(11) NULL COMMENT '赛事管理员 admin_user.id' AFTER school_id,
  ADD COLUMN creator_id INT(11) NULL COMMENT '创建赛事的管理员 admin_user.id' AFTER admin_id,
  ADD UNIQUE KEY uk_event_admin_id (admin_id),
  ADD KEY idx_event_creator_id (creator_id),
  ADD CONSTRAINT fk_event_admin FOREIGN KEY (admin_id) REFERENCES admin_user(id) ON DELETE SET NULL,
  ADD CONSTRAINT fk_event_creator FOREIGN KEY (creator_id) REFERENCES admin_user(id) ON DELETE SET NULL;

-- 如现网 role 有 CHECK 约束，需将其调整为包含 4；当前导出结构为整数列，无枚举变更。
