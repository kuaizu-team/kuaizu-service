-- Keep the earliest-created account for each phone or normalized email.
-- Later duplicate accounts remain intact; only the conflicting contact field is cleared.
-- This migration is intentionally not rerunnable.

UPDATE `user`
SET phone = NULLIF(TRIM(phone), ''),
    email = NULLIF(LOWER(TRIM(email)), '');

UPDATE `user` u
JOIN (
    SELECT id
    FROM (
        SELECT id,
               ROW_NUMBER() OVER (PARTITION BY phone ORDER BY created_at ASC, id ASC) AS row_num
        FROM `user`
        WHERE phone IS NOT NULL
    ) ranked_phone
    WHERE row_num > 1
) duplicate_phone ON duplicate_phone.id = u.id
SET u.phone = NULL;

UPDATE `user` u
JOIN (
    SELECT id
    FROM (
        SELECT id,
               ROW_NUMBER() OVER (PARTITION BY email ORDER BY created_at ASC, id ASC) AS row_num
        FROM `user`
        WHERE email IS NOT NULL
    ) ranked_email
    WHERE row_num > 1
) duplicate_email ON duplicate_email.id = u.id
SET u.email = NULL;

ALTER TABLE `user`
    ADD UNIQUE INDEX uq_user_phone (phone),
    ADD UNIQUE INDEX uq_user_email (email);
