-- Personal schedules v1. Run manually before deploying the API and Mini Program.
-- Asia/Shanghai wall-clock date/time. all_day=1 => schedule_time=NULL.
-- No existing table/data is modified; safe to run again for the initial rollout.
CREATE TABLE IF NOT EXISTS user_schedule (
 id BIGINT NOT NULL AUTO_INCREMENT,
 user_id INT NOT NULL,
 title VARCHAR(100) NOT NULL,
 schedule_date DATE NOT NULL,
 all_day TINYINT(1) NOT NULL DEFAULT 0,
 schedule_time TIME DEFAULT NULL,
 location VARCHAR(200) NOT NULL DEFAULT '',
 notes TEXT NOT NULL,
 created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
 PRIMARY KEY (id),
 KEY idx_user_schedule_date (user_id,schedule_date,schedule_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Read-only verification (expect the listed columns/index).
SHOW COLUMNS FROM user_schedule;
SHOW INDEX FROM user_schedule;
-- Do not drop this table on rollback: it contains users' private schedules.
