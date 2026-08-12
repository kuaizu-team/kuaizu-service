-- Users who have completed both school and major should have a talent profile
-- waiting for review. Existing profiles are deliberately not changed because
-- status=0 also represents rejection or a user who has left the talent pool,
-- while status=1 represents an already approved profile.
--
-- Safe to run repeatedly: user_id is unique and only missing profiles are
-- inserted; the NULL-status normalization has no effect after the first run.

START TRANSACTION;

INSERT INTO talent_profile (user_id, status)
SELECT u.id, 2
FROM `user` AS u
LEFT JOIN talent_profile AS tp ON tp.user_id = u.id
WHERE u.school_id IS NOT NULL
  AND u.major_id IS NOT NULL
  AND tp.id IS NULL;

UPDATE talent_profile AS tp
INNER JOIN `user` AS u ON u.id = tp.user_id
SET tp.status = 2,
    tp.reject_reason = NULL,
    tp.updated_at = CURRENT_TIMESTAMP
WHERE u.school_id IS NOT NULL
  AND u.major_id IS NOT NULL
  AND tp.status IS NULL;

COMMIT;
