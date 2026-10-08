-- READ ONLY. Use kuaizu_db; return every result set and any error.
-- No writes, no provider calls, no raw names/phones/emails are returned.
-- Explains threshold users versus normal-send candidates at THIS query time.
-- If automatic state rows appear, AUTO_STATE_NOT_READY may include read-only
-- recovery candidates; Q06 does not count those as a new provider send.
SELECT 'Q06_ENV' AS receipt,DATABASE() AS database_name,NOW() AS database_now,
 (SELECT COUNT(1) FROM auto_urge_sms_state) AS auto_state_rows;
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
) SELECT 'Q06_FILTER_REASONS' AS receipt,reason,COUNT(1) AS users
FROM (
 SELECT e.user_id,CASE
  WHEN u.id IS NULL THEN 'USER_NOT_FOUND'
  WHEN u.id<=0 THEN 'INVALID_USER_ID'
  WHEN u.user_status IS NULL THEN 'USER_STATUS_NULL'
  WHEN u.user_status=1 THEN 'USER_BANNED'
  WHEN NULLIF(TRIM(u.phone),'') IS NULL THEN 'PHONE_MISSING'
  WHEN a.user_id IS NOT NULL AND NOT (
   (a.state IN ('ready','failed') AND (a.next_retry_at IS NULL OR a.next_retry_at<=NOW()))
   OR (a.state='sent' AND a.closed_at IS NOT NULL)
  ) THEN 'AUTO_STATE_NOT_READY'
  ELSE 'NORMAL_SEND_CANDIDATE'
 END AS reason
 FROM eligible e LEFT JOIN `user` u ON u.id=e.user_id
 LEFT JOIN auto_urge_sms_state a ON a.user_id=e.user_id
) reasons GROUP BY reason ORDER BY reason;

-- Actually execute the per-user recheck for the sample from the previous receipt.
-- It can return no row if that user handled work since the previous snapshot.
WITH pending AS (
 SELECT ob.created_at AS pending_at FROM olive_branch_record ob
 JOIN project p ON p.id=ob.related_project_id
 WHERE ob.receiver_id=1128 AND ob.status=0 AND p.status<>4 AND p.deleted_at IS NULL
 UNION ALL
 SELECT pa.applied_at FROM project_application pa JOIN project p ON p.id=pa.project_id
 WHERE pa.status=0 AND p.status<>4 AND p.deleted_at IS NULL
 AND (p.creator_id=1128 OR EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=p.id AND pm.user_id=1128))
), eligible AS (
 SELECT COUNT(*) AS pending_count,MIN(pending_at) AS oldest_pending_at FROM pending
 HAVING COUNT(*)>=3 OR MIN(pending_at)<=DATE_SUB(NOW(),INTERVAL 168 HOUR)
)
SELECT 'Q07_RECHECK_RESULT' AS receipt,u.id AS user_id,e.pending_count,e.oldest_pending_at
FROM eligible e JOIN `user` u ON u.id=1128
WHERE u.user_status<>1 AND NULLIF(TRIM(u.phone),'') IS NOT NULL;
