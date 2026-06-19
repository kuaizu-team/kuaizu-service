-- Track only passive review transitions (pending -> approved/rejected) for
-- the one-time "my projects" red dot. Other project updates must not trigger it.
ALTER TABLE `project`
  ADD COLUMN `passive_status_changed_at` TIMESTAMP NULL DEFAULT NULL
  COMMENT 'Time when a pending project was passively reviewed as approved/rejected'
  AFTER `reject_reason`;
