-- 只读审计：识别手机号/邮箱重复账号及其核心业务资产。
-- 不修改数据。请回传结果后再制定逐组账号合并方案。

SELECT phone, COUNT(*) AS user_count, GROUP_CONCAT(id ORDER BY id) AS user_ids
FROM `user`
WHERE phone IS NOT NULL AND phone <> ''
GROUP BY phone
HAVING COUNT(*) > 1
ORDER BY user_count DESC, phone;

SELECT email, COUNT(*) AS user_count, GROUP_CONCAT(id ORDER BY id) AS user_ids
FROM `user`
WHERE email IS NOT NULL AND email <> ''
GROUP BY email
HAVING COUNT(*) > 1
ORDER BY user_count DESC, email;

SELECT
  u.id,
  u.phone,
  u.email,
  u.created_at,
  (SELECT COUNT(*) FROM `order` o WHERE o.user_id = u.id) AS order_count,
  (SELECT COUNT(*) FROM project p WHERE p.creator_id = u.id) AS created_project_count,
  (SELECT COUNT(*) FROM project_application pa WHERE pa.user_id = u.id) AS application_count,
  (SELECT COUNT(*) FROM talent_profile tp WHERE tp.user_id = u.id) AS talent_profile_count,
  (SELECT COUNT(*) FROM olive_branch_record obr WHERE obr.sender_id = u.id OR obr.receiver_id = u.id) AS olive_branch_count
FROM `user` u
WHERE u.phone IN (
  SELECT phone FROM `user`
  WHERE phone IS NOT NULL AND phone <> ''
  GROUP BY phone HAVING COUNT(*) > 1
)
OR u.email IN (
  SELECT email FROM `user`
  WHERE email IS NOT NULL AND email <> ''
  GROUP BY email HAVING COUNT(*) > 1
)
ORDER BY COALESCE(u.phone, ''), COALESCE(u.email, ''), u.id;
