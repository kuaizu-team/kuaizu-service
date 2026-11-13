-- Run once before deploying. Existing platform orders remain national.
ALTER TABLE commercial_order
  ADD COLUMN origin_school_id INT NULL,
  ADD COLUMN promotion_status VARCHAR(20) NOT NULL DEFAULT 'national' COMMENT 'local, submitted, national',
  ADD COLUMN promotion_consent_by INT NULL,
  ADD COLUMN promotion_consent_at DATETIME NULL,
  ADD KEY idx_commercial_order_origin (origin_school_id,promotion_status,created_at,id),
  ADD KEY idx_commercial_order_promotion (promotion_status,created_at,id),
  ADD CONSTRAINT fk_commercial_order_origin FOREIGN KEY (origin_school_id) REFERENCES school(id) ON DELETE RESTRICT;

SHOW COLUMNS FROM commercial_order LIKE 'promotion%';
SELECT promotion_status,COUNT(*) FROM commercial_order GROUP BY promotion_status;
