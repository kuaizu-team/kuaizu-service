-- Run once before deploying QR uploads. Existing ordinary images are unaffected.
ALTER TABLE campus_cooperation_image
  ADD COLUMN expires_at DATETIME NULL DEFAULT NULL COMMENT 'QR expiry; NULL for ordinary cooperation images',
  ADD KEY idx_cooperation_image_expiry (expires_at,object_key);
SHOW COLUMNS FROM campus_cooperation_image LIKE 'expires_at';
SHOW INDEX FROM campus_cooperation_image WHERE Key_name='idx_cooperation_image_expiry';
