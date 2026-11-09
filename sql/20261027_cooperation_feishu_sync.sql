-- Run on the intended MySQL 8 database before deploying the cooperation Feishu sync.
-- Existing orders, claims and applications are preserved.
DROP PROCEDURE IF EXISTS prepare_cooperation_feishu_sync;
DELIMITER $$
CREATE PROCEDURE prepare_cooperation_feishu_sync()
BEGIN
  IF DATABASE() IS NULL THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Select the intended database first';
  END IF;
  IF (SELECT COUNT(*) FROM information_schema.TABLES
      WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN
      ('commercial_order','campus_cooperation','campus_cooperation_application')) <> 3 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Campus cooperation schema is missing';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.COLUMNS
      WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='commercial_order'
        AND COLUMN_NAME='feishu_wiki_node_token') THEN
    ALTER TABLE commercial_order ADD COLUMN feishu_wiki_node_token VARCHAR(128) NULL
      COMMENT 'Bound commercial document in the 快组商业化 wiki';
  END IF;
END$$
DELIMITER ;
CALL prepare_cooperation_feishu_sync();
DROP PROCEDURE prepare_cooperation_feishu_sync;

CREATE TABLE cooperation_feishu_target (
  cooperation_id BIGINT NOT NULL,
  app_token VARCHAR(128) NULL,
  table_id VARCHAR(128) NULL,
  view_id VARCHAR(128) NULL,
  doc_block_id VARCHAR(128) NULL,
  status VARCHAR(24) NOT NULL DEFAULT 'idle',
  message VARCHAR(500) NOT NULL DEFAULT '',
  synced_at DATETIME NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (cooperation_id),
  CONSTRAINT fk_cooperation_feishu_target FOREIGN KEY (cooperation_id)
    REFERENCES campus_cooperation(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE cooperation_feishu_record (
  application_id BIGINT NOT NULL,
  cooperation_id BIGINT NOT NULL,
  record_id VARCHAR(128) NULL,
  client_token VARCHAR(128) NOT NULL,
  state VARCHAR(24) NOT NULL DEFAULT 'pending',
  PRIMARY KEY (application_id),
  KEY idx_cooperation_feishu_record_target (cooperation_id),
  CONSTRAINT fk_cooperation_feishu_record_target FOREIGN KEY (cooperation_id)
    REFERENCES cooperation_feishu_target(cooperation_id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
