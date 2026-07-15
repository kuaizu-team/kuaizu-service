-- 校区超级管理员多学校、唯一运营负责人及分权结算迁移（MySQL 8.0+）。
-- admin_user.school_id / commission_rate 暂时保留，仅供校区管理员、赛事管理员
-- 和旧版本客户端兼容；role=2 的权威数据迁移到 admin_school_relation。

CREATE TABLE IF NOT EXISTS admin_school_relation (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  admin_user_id INT NOT NULL COMMENT 'admin_user.id',
  school_id INT NOT NULL COMMENT 'school.id',
  commission_rate DECIMAL(5,2) NOT NULL DEFAULT 0.00 COMMENT '该管理员在该学校的结算比例（百分比）',
  is_owner TINYINT(1) NOT NULL DEFAULT 1 COMMENT '1=唯一运营负责人并拥有数据权限，0=仅保留结算权益',
  owner_school_id INT GENERATED ALWAYS AS (CASE WHEN is_owner = 1 THEN school_id ELSE NULL END) STORED,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_admin_school (admin_user_id, school_id),
  UNIQUE KEY uk_school_owner (owner_school_id),
  KEY idx_relation_school (school_id),
  CONSTRAINT fk_admin_school_admin FOREIGN KEY (admin_user_id) REFERENCES admin_user(id) ON DELETE CASCADE,
  CONSTRAINT fk_admin_school_school FOREIGN KEY (school_id) REFERENCES school(id) ON DELETE RESTRICT,
  CONSTRAINT chk_admin_school_rate CHECK (commission_rate >= 0 AND commission_rate <= 100)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='校区超级管理员学校权限与结算权益';

-- 历史 role=2 单学校管理员自动成为该学校负责人，沿用原分成比例。
-- 若历史脏数据中同一学校已有多个 role=2，先保留 id 最小者为负责人，
-- 其余记录作为仅结算权益迁移，避免迁移本身因唯一约束失败。
INSERT INTO admin_school_relation (admin_user_id, school_id, commission_rate, is_owner, created_at, updated_at)
SELECT au.id,
       au.school_id,
       CASE WHEN au.id = owners.owner_admin_id
            THEN LEAST(100.00, GREATEST(0.00, COALESCE(au.commission_rate, 0.00)))
            ELSE 0.00 END,
       CASE WHEN au.id = owners.owner_admin_id THEN 1 ELSE 0 END,
       au.created_at,
       au.updated_at
FROM admin_user au
JOIN (
  SELECT school_id, MIN(id) AS owner_admin_id
  FROM admin_user
  WHERE role = 2 AND school_id IS NOT NULL
  GROUP BY school_id
) owners ON owners.school_id = au.school_id
WHERE au.role = 2 AND au.school_id IS NOT NULL
ON DUPLICATE KEY UPDATE
  commission_rate = VALUES(commission_rate),
  updated_at = VALUES(updated_at);

-- 数据库层兜底：同一学校所有结算权益比例之和不得超过 100%。
DROP TRIGGER IF EXISTS trg_admin_school_rate_before_insert;
DROP TRIGGER IF EXISTS trg_admin_school_rate_before_update;
DELIMITER $$
CREATE TRIGGER trg_admin_school_rate_before_insert
BEFORE INSERT ON admin_school_relation
FOR EACH ROW
BEGIN
  IF (SELECT COALESCE(SUM(commission_rate), 0) FROM admin_school_relation WHERE school_id = NEW.school_id)
       + NEW.commission_rate > 100.00 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'school commission rate total exceeds 100%';
  END IF;
END$$

CREATE TRIGGER trg_admin_school_rate_before_update
BEFORE UPDATE ON admin_school_relation
FOR EACH ROW
BEGIN
  IF (SELECT COALESCE(SUM(commission_rate), 0) FROM admin_school_relation WHERE school_id = NEW.school_id AND id <> OLD.id)
       + NEW.commission_rate > 100.00 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'school commission rate total exceeds 100%';
  END IF;
END$$
DELIMITER ;

-- 结算批次明确记录“受益管理员”，operator_admin_id 继续表示平台操作人。
ALTER TABLE settlement_record
  ADD COLUMN beneficiary_admin_user_id INT NULL COMMENT '本批次分成受益管理员' AFTER school_id,
  ADD COLUMN commission_rate DECIMAL(5,2) NULL COMMENT '本批次采用的分成比例' AFTER beneficiary_admin_user_id,
  ADD KEY idx_settlement_beneficiary (beneficiary_admin_user_id),
  ADD CONSTRAINT fk_settlement_beneficiary FOREIGN KEY (beneficiary_admin_user_id) REFERENCES admin_user(id) ON DELETE SET NULL;

-- 历史批次按迁移后负责人回填受益人；无法唯一推断的记录保持 NULL。
UPDATE settlement_record sr
LEFT JOIN admin_school_relation rel
  ON rel.school_id = sr.school_id AND rel.is_owner = 1
SET sr.beneficiary_admin_user_id = rel.admin_user_id,
    sr.commission_rate = rel.commission_rate
WHERE sr.beneficiary_admin_user_id IS NULL;

ALTER TABLE settlement_record_order
  ADD COLUMN beneficiary_admin_user_id INT NULL COMMENT '分成受益管理员' AFTER order_id,
  ADD KEY idx_settlement_order_beneficiary (beneficiary_admin_user_id),
  ADD CONSTRAINT fk_settlement_order_beneficiary FOREIGN KEY (beneficiary_admin_user_id) REFERENCES admin_user(id) ON DELETE SET NULL;

UPDATE settlement_record_order sro
JOIN settlement_record sr ON sr.id = sro.settlement_record_id
SET sro.beneficiary_admin_user_id = sr.beneficiary_admin_user_id
WHERE sro.beneficiary_admin_user_id IS NULL;

ALTER TABLE settlement_record_order
  ADD UNIQUE KEY uk_order_beneficiary (order_id, beneficiary_admin_user_id);
