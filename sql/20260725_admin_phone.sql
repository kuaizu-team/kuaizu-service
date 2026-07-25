-- 管理员联系电话；历史数据允许为空，新建校验由接口负责。
ALTER TABLE admin_user ADD COLUMN phone VARCHAR(11) DEFAULT NULL COMMENT '联系电话' AFTER nickname;
