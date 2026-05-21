-- Dashboard v2 migration
-- Compatible with MySQL 5.7+.  Safe to re-run (all blocks are idempotent).

-- ─────────────────────────────────────────────────────────────────────────────
-- 1. Ensure project_view_log table exists.
--    CREATE TABLE IF NOT EXISTS is supported by MySQL 5.7+.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS `project_view_log` (
  `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
  `project_id`  INT             NOT NULL                COMMENT '项目ID',
  `user_id`     INT             DEFAULT NULL            COMMENT '访问用户ID（未登录为NULL）',
  `source`      TINYINT         NOT NULL DEFAULT 0      COMMENT '来源: 0-未知, 1-列表页, 2-分享卡片',
  `duration_ms` INT UNSIGNED    DEFAULT NULL            COMMENT '停留时长(毫秒)；NULL=纯浏览记录，非NULL=停留上报',
  `viewed_at`   TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '记录时间',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='项目浏览日志';

-- ─────────────────────────────────────────────────────────────────────────────
-- 2. Add duration_ms column if it does not yet exist.
--    Checks information_schema.columns to stay idempotent.
-- ─────────────────────────────────────────────────────────────────────────────
DROP PROCEDURE IF EXISTS _pvl_add_duration_ms;
DELIMITER $$
CREATE PROCEDURE _pvl_add_duration_ms()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name   = 'project_view_log'
      AND column_name  = 'duration_ms'
  ) THEN
    ALTER TABLE project_view_log
      ADD COLUMN `duration_ms` INT UNSIGNED DEFAULT NULL
        COMMENT '停留时长(毫秒)；NULL=纯浏览记录，非NULL=停留上报'
      AFTER `source`;
  END IF;
END$$
DELIMITER ;
CALL _pvl_add_duration_ms();
DROP PROCEDURE IF EXISTS _pvl_add_duration_ms;

-- ─────────────────────────────────────────────────────────────────────────────
-- 3. Performance indexes — checked via information_schema.statistics.
--    NOTE: MySQL does NOT support "CREATE INDEX IF NOT EXISTS" (any version).
--          MariaDB does, MySQL does not.  Use stored procedures instead.
--
--    idx_pvl_project_viewed_at (project_id, viewed_at)
--      → today_views, hourly_views, viewers queries
--    idx_pvl_project_user (project_id, user_id)
--      → viewers de-dup / GROUP BY query
-- ─────────────────────────────────────────────────────────────────────────────
DROP PROCEDURE IF EXISTS _pvl_create_indexes;
DELIMITER $$
CREATE PROCEDURE _pvl_create_indexes()
BEGIN
  -- idx_pvl_project_viewed_at
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name   = 'project_view_log'
      AND index_name   = 'idx_pvl_project_viewed_at'
  ) THEN
    CREATE INDEX idx_pvl_project_viewed_at ON project_view_log (project_id, viewed_at);
  END IF;

  -- idx_pvl_project_user
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name   = 'project_view_log'
      AND index_name   = 'idx_pvl_project_user'
  ) THEN
    CREATE INDEX idx_pvl_project_user ON project_view_log (project_id, user_id);
  END IF;
END$$
DELIMITER ;
CALL _pvl_create_indexes();
DROP PROCEDURE IF EXISTS _pvl_create_indexes;
