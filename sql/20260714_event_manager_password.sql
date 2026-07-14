-- 赛事管理员可查看密码支持。
-- 原 password_hash 继续用于登录校验；该列只保存 AES-GCM 密文，并只在授权详情接口解密。
-- 历史账号无法从 bcrypt 哈希还原密码，重置一次密码后会自动写入本列。
ALTER TABLE admin_user
  ADD COLUMN password_encrypted TEXT NULL COMMENT '赛事管理员密码 AES-GCM 密文' AFTER password_hash;
