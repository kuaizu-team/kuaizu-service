-- Add sender/operator project role to olive branch records so clients can display
-- the actual role that sent/operated the invitation instead of falling back to 项目方.

DROP PROCEDURE IF EXISTS _olive_branch_operator_role;
DELIMITER $$
CREATE PROCEDURE _olive_branch_operator_role()
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'olive_branch_record'
      AND column_name = 'operator_role'
  ) THEN
    ALTER TABLE olive_branch_record
      ADD COLUMN operator_role VARCHAR(32) NULL AFTER status,
      ADD KEY idx_olive_operator_role (operator_role);
  END IF;

  UPDATE olive_branch_record ob
  JOIN project p ON p.id = ob.related_project_id
  LEFT JOIN project_members pm ON pm.project_id = ob.related_project_id AND pm.user_id = ob.sender_id
  SET ob.operator_role = COALESCE(NULLIF(ob.operator_role, ''), pm.role, p.publisher_role, 'TEAM_LEADER')
  WHERE ob.operator_role IS NULL OR ob.operator_role = '';
END$$
DELIMITER ;
CALL _olive_branch_operator_role();
DROP PROCEDURE IF EXISTS _olive_branch_operator_role;
