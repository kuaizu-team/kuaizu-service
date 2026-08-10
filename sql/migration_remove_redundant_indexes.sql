-- The matching UNIQUE indexes already cover these exact single-column lookups.
-- This migration is intentionally not rerunnable.
ALTER TABLE admin_user DROP INDEX idx_admin_user_username;
ALTER TABLE talent_profile DROP INDEX idx_talent_user;
ALTER TABLE `user` DROP INDEX idx_user_openid;
