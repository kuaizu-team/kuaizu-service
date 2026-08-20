-- Preview and remove project member rows whose user has already been deleted.
-- Review the SELECT result before executing the DELETE in production.
-- The 2026-08-17 export contains 6 such rows, all for deleted user ID 2351.

SELECT pm.id, pm.project_id, pm.user_id, pm.role, pm.created_at
FROM project_members pm
LEFT JOIN `user` u ON u.id = pm.user_id
WHERE u.id IS NULL
ORDER BY pm.project_id, pm.id;

DELETE pm
FROM project_members pm
LEFT JOIN `user` u ON u.id = pm.user_id
WHERE u.id IS NULL;
