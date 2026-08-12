-- Read-only production evidence for index review. Save every result set with the
-- release record. Run only after the verification scripts PASS.

SHOW INDEX FROM admin_user;
SHOW INDEX FROM `user`;
SHOW INDEX FROM project_view_log;
SHOW INDEX FROM talent_view_log;
SHOW INDEX FROM talent_profile;
SHOW INDEX FROM email_task;
SHOW INDEX FROM email_promotion;

SET @audit_project_id = COALESCE((SELECT project_id FROM project_view_log ORDER BY id DESC LIMIT 1), 0);
SET @audit_talent_id = COALESCE((SELECT talent_id FROM talent_view_log ORDER BY id DESC LIMIT 1), 0);
SET @audit_promotion_id = COALESCE((SELECT id FROM email_promotion ORDER BY id DESC LIMIT 1), 0);

-- Production is MySQL 8.0.13. EXPLAIN ANALYZE is unavailable there, so collect
-- the optimizer's JSON plan without executing the query.
EXPLAIN FORMAT=JSON
SELECT COUNT(DISTINCT vl.user_id)
FROM project_view_log vl
WHERE vl.project_id = @audit_project_id
  AND vl.user_id IS NOT NULL
  AND vl.viewed_at >= NOW() - INTERVAL 24 HOUR
  AND vl.duration_ms IS NULL;

EXPLAIN FORMAT=JSON
SELECT vl.user_id, MAX(vl.viewed_at) AS last_viewed_at
FROM project_view_log vl
WHERE vl.project_id = @audit_project_id
  AND vl.user_id IS NOT NULL
  AND vl.viewed_at >= NOW() - INTERVAL 24 HOUR
  AND vl.duration_ms IS NULL
GROUP BY vl.user_id
ORDER BY last_viewed_at DESC, vl.user_id DESC
LIMIT 20 OFFSET 20;

EXPLAIN FORMAT=JSON
SELECT vl.user_id, MAX(vl.viewed_at) AS last_viewed_at
FROM talent_view_log vl
WHERE vl.talent_id = @audit_talent_id
  AND vl.user_id IS NOT NULL
  AND vl.viewed_at >= NOW() - INTERVAL 24 HOUR
  AND vl.duration_ms IS NULL
GROUP BY vl.user_id
ORDER BY last_viewed_at DESC, vl.user_id DESC
LIMIT 20 OFFSET 20;

EXPLAIN FORMAT=JSON
SELECT status, COUNT(*)
FROM email_task
WHERE promotion_id = @audit_promotion_id AND channel = 'EMAIL'
GROUP BY status;

EXPLAIN FORMAT=JSON
SELECT id
FROM email_promotion
WHERE channel = 'EMAIL' AND business_tag = 'project_promotion'
  AND status IN (0, 1)
ORDER BY id
LIMIT 100;
