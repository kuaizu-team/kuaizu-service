-- 官网团队展示资料。所有字段均可空，避免影响现有管理员账号。
ALTER TABLE admin_user
  ADD COLUMN join_date DATE NULL COMMENT '加入快组日期' AFTER commission_rate,
  ADD COLUMN intro VARCHAR(255) NULL COMMENT '官网团队一句话介绍' AFTER join_date,
  ADD COLUMN article_url VARCHAR(500) NULL COMMENT '公众号个人或团队故事链接' AFTER intro;
