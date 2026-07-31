-- Backfill real-time collaboration scores from frozen history and active projects.

UPDATE `user` u
LEFT JOIN (
  SELECT scores.user_id, AVG(scores.score) AS score
  FROM (
    SELECT cs.user_id, cs.score
    FROM collaboration_score cs
    UNION ALL
    SELECT pms.member_id AS user_id, pms.score
    FROM project_member_score pms
    INNER JOIN project_members pm ON pm.id = pms.project_member_id
    WHERE pms.score IS NOT NULL
  ) scores
  GROUP BY scores.user_id
) calculated ON calculated.user_id = u.id
SET u.collaboration_score = COALESCE(calculated.score, 90.00);