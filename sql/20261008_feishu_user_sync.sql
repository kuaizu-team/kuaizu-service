-- Apply manually before enabling FEISHU_USER_SYNC_ENABLED. No existing user rows
-- are changed; intentionally no cascading foreign keys: deleted users retain a
-- mapping until their system-created remote record is successfully removed.
CREATE TABLE IF NOT EXISTS feishu_user_sync_target (
  school_id INT NOT NULL PRIMARY KEY,
  app_token VARCHAR(128) NOT NULL DEFAULT '',
  node_token VARCHAR(128) NOT NULL DEFAULT '',
  table_id VARCHAR(128) NOT NULL DEFAULT '',
  view_id VARCHAR(128) NOT NULL DEFAULT '',
  node_started TINYINT NOT NULL DEFAULT 0,
  table_started TINYINT NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS feishu_user_sync_job (
  id CHAR(36) NOT NULL PRIMARY KEY,
  school_id INT NOT NULL,
  admin_id INT NOT NULL,
  active_school_id INT NULL,
  status VARCHAR(24) NOT NULL,
  total INT NOT NULL DEFAULT 0,
  processed INT NOT NULL DEFAULT 0,
  created_count INT NOT NULL DEFAULT 0,
  updated_count INT NOT NULL DEFAULT 0,
  deleted_count INT NOT NULL DEFAULT 0,
  message VARCHAR(1024) NOT NULL DEFAULT '',
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  UNIQUE KEY uk_feishu_sync_active_school (active_school_id),
  KEY idx_feishu_sync_school_created (school_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS feishu_user_sync_record (
  school_id INT NOT NULL,
  user_id INT NOT NULL,
  record_id VARCHAR(128) NOT NULL DEFAULT '',
  client_token CHAR(36) NOT NULL,
  state VARCHAR(24) NOT NULL,
  payload MEDIUMTEXT NULL,
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (school_id,user_id),
  UNIQUE KEY uk_feishu_sync_client_token (client_token),
  KEY idx_feishu_sync_record_id (school_id,record_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
