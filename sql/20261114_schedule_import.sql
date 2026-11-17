-- Execute ONCE, after 20261113_user_schedule.sql, before deploying course import.
-- Preflight: SHOW COLUMNS FROM user_schedule LIKE 'import_key';
-- If import_key already exists, verify its index below instead of repeating ALTER.
-- Existing manual schedules keep NULL and are not changed by import deduplication.
ALTER TABLE user_schedule
  ADD COLUMN import_key CHAR(64) DEFAULT NULL COMMENT 'SHA256 of normalized imported schedule',
  ADD UNIQUE KEY uk_user_schedule_import (user_id, import_key);

SHOW COLUMNS FROM user_schedule LIKE 'import_key';
SHOW INDEX FROM user_schedule WHERE Key_name = 'uk_user_schedule_import';
-- Keep column/index on application rollback; no user data needs to be deleted.
