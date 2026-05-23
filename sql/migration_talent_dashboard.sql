-- Talent dashboard migration for MySQL 5.7.
-- Run once. MySQL 5.7 does not support CREATE INDEX IF NOT EXISTS.

ALTER TABLE talent_profile
  ADD COLUMN view_count INT NOT NULL DEFAULT 0 COMMENT '累计浏览量';

CREATE TABLE talent_view_log (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  talent_id   INT NOT NULL COMMENT '名片ID',
  user_id     INT DEFAULT NULL COMMENT '查看者用户ID（未登录为NULL）',
  source      TINYINT NOT NULL DEFAULT 0 COMMENT '0未知,1人才库列表,2分享卡片',
  duration_ms INT UNSIGNED DEFAULT NULL COMMENT '停留毫秒数（NULL表示浏览记录）',
  viewed_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '查看时间',
  INDEX idx_talent_viewed_at (talent_id, viewed_at),
  INDEX idx_talent_user (talent_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='名片浏览日志';
