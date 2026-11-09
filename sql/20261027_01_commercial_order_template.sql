-- Run once on the intended database before deploying the cooperation changes.
-- Existing commercial orders and campus cooperation/application rows are preserved.
ALTER TABLE commercial_order
  ADD COLUMN content_template JSON NULL COMMENT 'Editable draft content copied by each school after claiming' AFTER description;

ALTER TABLE commercial_order
  MODIFY COLUMN status VARCHAR(20) NOT NULL DEFAULT 'available' COMMENT 'available: new claims; closed: existing schools continue; ended: all campus publications withdrawn';

SHOW COLUMNS FROM commercial_order LIKE 'content_template';
