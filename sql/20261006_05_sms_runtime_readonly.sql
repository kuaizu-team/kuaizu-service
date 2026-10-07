-- READ ONLY. Select kuaizu_db. AUTO_URGE_SMS_ENABLED remains false.
-- Generated from the current Go candidate and recheck queries; no provider calls.
-- No INSERT/UPDATE/DELETE/ALTER and no deployment/configuration changes.
-- Capture ALL result sets, plus any errors. No phone/email/openid is selected.
SELECT 'Q01_ENV' AS receipt,DATABASE() AS database_name,VERSION() AS mysql_version,
 NOW() AS database_now,@@session.time_zone AS session_time_zone;
SELECT 'Q02_STATE_TOTAL' AS receipt,COUNT(1) AS records FROM auto_urge_sms_state;

-- All historical unresolved records are included. This is only a preview.
WITH reviewers AS (
 SELECT id AS project_id,creator_id AS user_id FROM project WHERE status<>4 AND deleted_at IS NULL
 UNION
 SELECT pm.project_id,pm.user_id FROM project_members pm
 JOIN project p ON p.id=pm.project_id WHERE p.status<>4 AND p.deleted_at IS NULL
), pending AS (
 SELECT ob.receiver_id AS user_id,ob.created_at AS pending_at FROM olive_branch_record ob
 JOIN project p ON p.id=ob.related_project_id WHERE ob.status=0 AND p.status<>4 AND p.deleted_at IS NULL
 UNION ALL
 SELECT r.user_id,pa.applied_at AS pending_at FROM project_application pa
 JOIN reviewers r ON r.project_id=pa.project_id WHERE pa.status=0
), eligible AS (
 SELECT user_id,COUNT(*) AS pending_count,MIN(pending_at) AS oldest_pending_at FROM pending GROUP BY user_id
 HAVING COUNT(*)>=3 OR MIN(pending_at)<=DATE_SUB(NOW(),INTERVAL 168 HOUR)
)
SELECT 'Q03_ELIGIBLE_TOTAL' AS receipt,COUNT(1) AS eligible_users,
 COALESCE(SUM(pending_count),0) AS pending_records,
 COALESCE(SUM(pending_count>=3),0) AS count_threshold_users,
 COALESCE(SUM(oldest_pending_at<=DATE_SUB(NOW(),INTERVAL 168 HOUR)),0) AS age_threshold_users
FROM eligible;

-- Exact runtime candidate SQL, first page: plan only, no recipient data returned.
EXPLAIN
WITH reviewers AS (
 SELECT id AS project_id,creator_id AS user_id FROM project WHERE status<>4 AND deleted_at IS NULL
 UNION
 SELECT pm.project_id,pm.user_id FROM project_members pm
 JOIN project p ON p.id=pm.project_id WHERE p.status<>4 AND p.deleted_at IS NULL
), pending AS (
 SELECT ob.receiver_id AS user_id,ob.created_at AS pending_at FROM olive_branch_record ob
 JOIN project p ON p.id=ob.related_project_id WHERE ob.status=0 AND p.status<>4 AND p.deleted_at IS NULL
 UNION ALL
 SELECT r.user_id,pa.applied_at AS pending_at FROM project_application pa
 JOIN reviewers r ON r.project_id=pa.project_id WHERE pa.status=0
), eligible AS (
 SELECT user_id,COUNT(*) AS pending_count,MIN(pending_at) AS oldest_pending_at FROM pending GROUP BY user_id
 HAVING COUNT(*)>=3 OR MIN(pending_at)<=DATE_SUB(NOW(),INTERVAL 168 HOUR)
)
 SELECT u.id AS user_id,COALESCE(NULLIF(TRIM(u.nickname),''),'快组儿') AS nickname,
 COALESCE(e.pending_count,0) AS pending_count,e.oldest_pending_at
 FROM `user` u LEFT JOIN eligible e ON e.user_id=u.id
 LEFT JOIN auto_urge_sms_state a ON a.user_id=u.id
 WHERE u.id>0 AND (
 (e.user_id IS NOT NULL AND u.user_status<>1 AND NULLIF(TRIM(u.phone),'') IS NOT NULL
 AND (a.user_id IS NULL OR (a.state IN ('ready','failed') AND (a.next_retry_at IS NULL OR a.next_retry_at<=NOW()))
 OR (a.state='sent' AND a.closed_at IS NOT NULL)))
 OR (a.request_key IS NOT NULL AND
 ((a.state='unknown' AND (a.next_retry_at IS NULL OR a.next_retry_at<=NOW()))
 OR (a.state='claimed' AND a.last_attempt_at<=DATE_SUB(NOW(),INTERVAL 30 MINUTE)))))
 ORDER BY u.id LIMIT 200;

-- Count the first runtime page without exporting names or contacts.
WITH reviewers AS (
 SELECT id AS project_id,creator_id AS user_id FROM project WHERE status<>4 AND deleted_at IS NULL
 UNION
 SELECT pm.project_id,pm.user_id FROM project_members pm
 JOIN project p ON p.id=pm.project_id WHERE p.status<>4 AND p.deleted_at IS NULL
), pending AS (
 SELECT ob.receiver_id AS user_id,ob.created_at AS pending_at FROM olive_branch_record ob
 JOIN project p ON p.id=ob.related_project_id WHERE ob.status=0 AND p.status<>4 AND p.deleted_at IS NULL
 UNION ALL
 SELECT r.user_id,pa.applied_at AS pending_at FROM project_application pa
 JOIN reviewers r ON r.project_id=pa.project_id WHERE pa.status=0
), eligible AS (
 SELECT user_id,COUNT(*) AS pending_count,MIN(pending_at) AS oldest_pending_at FROM pending GROUP BY user_id
 HAVING COUNT(*)>=3 OR MIN(pending_at)<=DATE_SUB(NOW(),INTERVAL 168 HOUR)
)
SELECT 'Q04_FIRST_PAGE_TOTAL' AS receipt,COUNT(1) AS candidates
FROM (
 SELECT u.id AS user_id,COALESCE(NULLIF(TRIM(u.nickname),''),'快组儿') AS nickname,
 COALESCE(e.pending_count,0) AS pending_count,e.oldest_pending_at
 FROM `user` u LEFT JOIN eligible e ON e.user_id=u.id
 LEFT JOIN auto_urge_sms_state a ON a.user_id=u.id
 WHERE u.id>0 AND (
 (e.user_id IS NOT NULL AND u.user_status<>1 AND NULLIF(TRIM(u.phone),'') IS NOT NULL
 AND (a.user_id IS NULL OR (a.state IN ('ready','failed') AND (a.next_retry_at IS NULL OR a.next_retry_at<=NOW()))
 OR (a.state='sent' AND a.closed_at IS NOT NULL)))
 OR (a.request_key IS NOT NULL AND
 ((a.state='unknown' AND (a.next_retry_at IS NULL OR a.next_retry_at<=NOW()))
 OR (a.state='claimed' AND a.last_attempt_at<=DATE_SUB(NOW(),INTERVAL 30 MINUTE)))))
 ORDER BY u.id LIMIT 200) preview;

-- Select one eligible user for the exact per-recipient recheck plan.
WITH reviewers AS (
 SELECT id AS project_id,creator_id AS user_id FROM project WHERE status<>4 AND deleted_at IS NULL
 UNION
 SELECT pm.project_id,pm.user_id FROM project_members pm
 JOIN project p ON p.id=pm.project_id WHERE p.status<>4 AND p.deleted_at IS NULL
), pending AS (
 SELECT ob.receiver_id AS user_id,ob.created_at AS pending_at FROM olive_branch_record ob
 JOIN project p ON p.id=ob.related_project_id WHERE ob.status=0 AND p.status<>4 AND p.deleted_at IS NULL
 UNION ALL
 SELECT r.user_id,pa.applied_at AS pending_at FROM project_application pa
 JOIN reviewers r ON r.project_id=pa.project_id WHERE pa.status=0
), eligible AS (
 SELECT user_id,COUNT(*) AS pending_count,MIN(pending_at) AS oldest_pending_at FROM pending GROUP BY user_id
 HAVING COUNT(*)>=3 OR MIN(pending_at)<=DATE_SUB(NOW(),INTERVAL 168 HOUR)
)
SELECT COALESCE(MIN(user_id),0) INTO @kz_audit_sms_user FROM eligible;
EXPLAIN
WITH pending AS (
 SELECT ob.created_at AS pending_at FROM olive_branch_record ob JOIN project p ON p.id=ob.related_project_id
 WHERE ob.receiver_id=@kz_audit_sms_user AND ob.status=0 AND p.status<>4 AND p.deleted_at IS NULL
 UNION ALL
 SELECT pa.applied_at FROM project_application pa JOIN project p ON p.id=pa.project_id
 WHERE pa.status=0 AND p.status<>4 AND p.deleted_at IS NULL AND (p.creator_id=@kz_audit_sms_user OR EXISTS (
 SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=@kz_audit_sms_user))
), eligible AS (SELECT COUNT(*) AS pending_count,MIN(pending_at) AS oldest_pending_at FROM pending
 HAVING COUNT(*)>=3 OR MIN(pending_at)<=DATE_SUB(NOW(),INTERVAL 168 HOUR))
 SELECT u.id AS user_id,COALESCE(NULLIF(TRIM(u.nickname),''),'快组儿') AS nickname,e.pending_count,e.oldest_pending_at
 FROM eligible e JOIN `user` u ON u.id=@kz_audit_sms_user WHERE u.user_status<>1 AND NULLIF(TRIM(u.phone),'') IS NOT NULL;
SELECT 'Q05_RECHECK_SAMPLE' AS receipt,@kz_audit_sms_user AS sample_user_id;
-- If sample_user_id=0, there was no eligible user; the query plan still verifies syntax.
