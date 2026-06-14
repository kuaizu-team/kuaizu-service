-- Records when a user last opened the "my projects" page.
-- Used by /users/me/project-status-unread to show a red dot for passive
-- review status changes (approved/rejected) that happened after this time.
ALTER TABLE `user`
  ADD COLUMN `last_viewed_my_projects_at` TIMESTAMP NULL DEFAULT NULL
  COMMENT 'Last time the user viewed my projects page'
  AFTER `applications_last_viewed_at`;
