ALTER TABLE talent_profile
  ADD COLUMN reject_reason VARCHAR(500) DEFAULT NULL COMMENT '驳回/下架原因';
