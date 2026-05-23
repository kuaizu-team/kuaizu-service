-- Talent dashboard migration for MySQL 5.7+.
-- Safe to re-run. MySQL 5.7 does not support ADD COLUMN IF NOT EXISTS
-- or CREATE INDEX IF NOT EXISTS, so information_schema checks are used.

DROP PROCEDURE IF EXISTS _talent_dashboard_add_view_count;
DELIMITER $$
CREATE PROCEDURE _talent_dashboard_add_view_count()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'talent_profile'
      AND column_name = 'view_count'
  ) THEN
    ALTER TABLE talent_profile
      ADD COLUMN view_count INT NOT NULL DEFAULT 0 COMMENT '累计浏览量';
  END IF;
END$$
DELIMITER ;
CALL _talent_dashboard_add_view_count();
DROP PROCEDURE IF EXISTS _talent_dashboard_add_view_count;

CREATE TABLE IF NOT EXISTS talent_view_log (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  talent_id   INT NOT NULL COMMENT '名片ID',
  user_id     INT DEFAULT NULL COMMENT '查看者用户ID（未登录为NULL）',
  source      TINYINT NOT NULL DEFAULT 0 COMMENT '0未知,1人才库列表,2分享卡片',
  duration_ms INT UNSIGNED DEFAULT NULL COMMENT '停留毫秒数（NULL表示浏览记录）',
  viewed_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '查看时间'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='名片浏览日志';

DROP PROCEDURE IF EXISTS _talent_dashboard_create_indexes;
DELIMITER $$
CREATE PROCEDURE _talent_dashboard_create_indexes()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'talent_view_log'
      AND index_name = 'idx_talent_viewed_at'
  ) THEN
    CREATE INDEX idx_talent_viewed_at ON talent_view_log (talent_id, viewed_at);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'talent_view_log'
      AND index_name = 'idx_talent_user'
  ) THEN
    CREATE INDEX idx_talent_user ON talent_view_log (talent_id, user_id);
  END IF;
END$$
DELIMITER ;
CALL _talent_dashboard_create_indexes();
DROP PROCEDURE IF EXISTS _talent_dashboard_create_indexes;
