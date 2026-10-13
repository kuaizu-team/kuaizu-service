-- MySQL 5.7+. Review and run manually BEFORE deploying the backend.
-- This script does not change status_notification or its priority/delivery flags.
-- Each source key is idempotent. Re-running never resets runtime read/display state.
-- Historical read state is assumed read; actual full-screen display remains unknown.
CREATE TABLE IF NOT EXISTS user_status_message (
  id BIGINT NOT NULL AUTO_INCREMENT,
  user_id INT NOT NULL,
  type VARCHAR(50) NOT NULL,
  source_key VARCHAR(160) NOT NULL COMMENT 'idempotency key within this user',
  status_notification_id BIGINT NULL COMMENT 'legacy queue link, deliberately no FK',
  payload JSON NULL COMMENT 'immutable display parameters; no contacts or credentials',
  read_at DATETIME NULL DEFAULT NULL,
  fullscreen_displayed TINYINT(1) NULL DEFAULT NULL COMMENT 'NULL historical unknown, 0 never, 1 displayed',
  fullscreen_displayed_at DATETIME NULL DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_status_message_source (user_id,source_key),
  UNIQUE KEY uk_status_message_notification (user_id,status_notification_id),
  KEY idx_status_message_unread (user_id,read_at,created_at,id),
  KEY idx_status_message_time (user_id,created_at,id),
  KEY idx_status_message_type (user_id,type,created_at,id),
  CONSTRAINT fk_status_message_user FOREIGN KEY (user_id) REFERENCES `user`(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='User status message archive independent of delivery queue';

START TRANSACTION;
INSERT INTO user_status_message(user_id,type,source_key,status_notification_id,payload,created_at,read_at)
SELECT sn.user_id,sn.type,CONCAT('status:',sn.id),sn.id,
  JSON_OBJECT('projectId',p.id,'projectName',p.name,'nickname',u.nickname,'projectCreatedAt',p.created_at,
  'appliedAt',COALESCE(pa.applied_at,ob.created_at,pmr.joined_at),
  'discussingAt',COALESCE(pa.discussing_at,ob.discussing_at),
  'rejectedAt',COALESCE(pa.rejected_at,ob.rejected_at),'joinedAt',COALESCE(pa.joined_at,ob.admitted_at),
  'reviewerRoleName',COALESCE(rr.name,obr.name),'assignedRoleName',COALESCE(ar.name,oar.name,pmrr.name),
  'operatorName',sender.nickname,'removedAt',pmr.removed_at,'eventAt',sn.created_at),
  sn.created_at,CURRENT_TIMESTAMP
FROM status_notification sn
LEFT JOIN project_application pa ON pa.id=sn.application_id
LEFT JOIN olive_branch_record ob ON ob.id=sn.olive_branch_id
LEFT JOIN project_member_removal pmr ON pmr.id=sn.member_removal_id
LEFT JOIN project p ON p.id=COALESCE(pa.project_id,ob.related_project_id,pmr.project_id)
LEFT JOIN project_role rr ON rr.code=pa.reviewer_role
LEFT JOIN project_role ar ON ar.code=pa.assigned_role
LEFT JOIN project_role obr ON obr.code=ob.operator_role
LEFT JOIN project_role oar ON oar.code=ob.assigned_role
LEFT JOIN project_role pmrr ON pmrr.code=pmr.role
LEFT JOIN `user` u ON u.id=sn.user_id
LEFT JOIN `user` sender ON sender.id=ob.sender_id
ON DUPLICATE KEY UPDATE source_key=VALUES(source_key);

INSERT INTO user_status_message(user_id,type,source_key,payload,created_at,read_at)
SELECT pa.user_id,'application-discussing',CONCAT('discussing:application:',pa.id),
  JSON_OBJECT('projectId',p.id,'projectName',p.name,'nickname',u.nickname,'appliedAt',pa.applied_at,
  'discussingAt',pa.discussing_at,'eventAt',pa.discussing_at,'reviewerRoleName',pr.name,'context','application'),
  pa.discussing_at,CURRENT_TIMESTAMP
FROM project_application pa LEFT JOIN project p ON p.id=pa.project_id
LEFT JOIN project_role pr ON pr.code=pa.reviewer_role LEFT JOIN `user` u ON u.id=pa.user_id
WHERE pa.discussing_at IS NOT NULL
ON DUPLICATE KEY UPDATE source_key=VALUES(source_key);

INSERT INTO user_status_message(user_id,type,source_key,payload,created_at,read_at)
SELECT ob.receiver_id,'application-discussing',CONCAT('discussing:olive:',ob.id),
  JSON_OBJECT('projectId',p.id,'projectName',p.name,'nickname',u.nickname,'appliedAt',ob.created_at,
  'discussingAt',ob.discussing_at,'eventAt',ob.discussing_at,'reviewerRoleName',pr.name,'context','olive'),
  ob.discussing_at,CURRENT_TIMESTAMP
FROM olive_branch_record ob LEFT JOIN project p ON p.id=ob.related_project_id
LEFT JOIN project_role pr ON pr.code=ob.operator_role LEFT JOIN `user` u ON u.id=ob.receiver_id
WHERE ob.discussing_at IS NOT NULL
ON DUPLICATE KEY UPDATE source_key=VALUES(source_key);

-- Only the available last recruit/end timestamps can be recovered, not lost cycles.
INSERT INTO user_status_message(user_id,type,source_key,payload,created_at,read_at)
SELECT p.creator_id,'project-published',CONCAT('project-published:',p.id),
  JSON_OBJECT('projectId',p.id,'projectName',p.name,'nickname',u.nickname,'projectCreatedAt',p.created_at,'eventAt',p.created_at),
  p.created_at,CURRENT_TIMESTAMP FROM project p LEFT JOIN `user` u ON u.id=p.creator_id
WHERE p.created_at IS NOT NULL
ON DUPLICATE KEY UPDATE source_key=VALUES(source_key);

INSERT INTO user_status_message(user_id,type,source_key,payload,created_at,read_at)
SELECT p.creator_id,'recruit-completed',CONCAT('recruit-completed:',p.id,':',UNIX_TIMESTAMP(p.recruit_completed_at)),
  JSON_OBJECT('projectId',p.id,'projectName',p.name,'nickname',u.nickname,'projectCreatedAt',p.created_at,
  'recruitCompletedAt',p.recruit_completed_at,'eventAt',p.recruit_completed_at,
  'totalViews',p.view_count,'likeCount',(SELECT COUNT(*) FROM project_like WHERE project_id=p.id),
  'favoriteCount',(SELECT COUNT(*) FROM project_favorite WHERE project_id=p.id),
  'shareCount',(SELECT COUNT(*) FROM project_share WHERE project_id=p.id)),
  p.recruit_completed_at,CURRENT_TIMESTAMP FROM project p LEFT JOIN `user` u ON u.id=p.creator_id
WHERE p.recruit_completed_at IS NOT NULL
ON DUPLICATE KEY UPDATE source_key=VALUES(source_key);

INSERT INTO user_status_message(user_id,type,source_key,payload,created_at,read_at)
SELECT p.creator_id,'project-ended',CONCAT('project-ended:',p.id),
  JSON_OBJECT('projectId',p.id,'projectName',p.name,'nickname',u.nickname,'projectCreatedAt',p.created_at,
  'endedAt',p.ended_at,'eventAt',p.ended_at,
  'totalViews',p.view_count,'likeCount',(SELECT COUNT(*) FROM project_like WHERE project_id=p.id),
  'favoriteCount',(SELECT COUNT(*) FROM project_favorite WHERE project_id=p.id),
  'shareCount',(SELECT COUNT(*) FROM project_share WHERE project_id=p.id)),p.ended_at,CURRENT_TIMESTAMP
FROM project p LEFT JOIN `user` u ON u.id=p.creator_id WHERE p.ended_at IS NOT NULL
ON DUPLICATE KEY UPDATE source_key=VALUES(source_key);

-- An existing profile proves a first submission, but not every resubmission.
INSERT INTO user_status_message(user_id,type,source_key,payload,created_at,read_at)
SELECT t.user_id,'talent-published',CONCAT('talent-published:history:',t.id),
  JSON_OBJECT('nickname',u.nickname,'eventAt',t.created_at),t.created_at,CURRENT_TIMESTAMP
FROM talent_profile t LEFT JOIN `user` u ON u.id=t.user_id
LEFT JOIN user_status_message m ON m.user_id=t.user_id AND m.type='talent-published'
WHERE t.created_at IS NOT NULL AND m.id IS NULL
ON DUPLICATE KEY UPDATE source_key=VALUES(source_key);

INSERT INTO user_status_message(user_id,type,source_key,payload,created_at,read_at)
SELECT pi.user_id,'super-admin-invitation',CONCAT('invitation:',pi.id,':',UNIX_TIMESTAMP(pi.expire_at)),
  JSON_OBJECT('nickname',u.nickname,'schoolName',s.school_name,'eventAt',pi.updated_at),pi.updated_at,CURRENT_TIMESTAMP
FROM pending_invitation pi LEFT JOIN `user` u ON u.id=pi.user_id LEFT JOIN school s ON s.id=u.school_id
WHERE pi.updated_at IS NOT NULL
ON DUPLICATE KEY UPDATE source_key=VALUES(source_key);

-- Cleared/expired invitation flags cannot be reconstructed exactly. Recover
-- at most one historical invitation from available feedback per user.
INSERT INTO user_status_message(user_id,type,source_key,payload,created_at,read_at)
SELECT f.user_id,'super-admin-invitation',CONCAT('invitation:history:',f.id),
  JSON_OBJECT('nickname',u.nickname,'schoolName',s.school_name,'eventAt',COALESCE(f.created_at,f.updated_at)),
  COALESCE(f.created_at,f.updated_at),CURRENT_TIMESTAMP
FROM invitation_feedback f LEFT JOIN `user` u ON u.id=f.user_id LEFT JOIN school s ON s.id=u.school_id
LEFT JOIN user_status_message m ON m.user_id=f.user_id AND m.type='super-admin-invitation'
WHERE COALESCE(f.created_at,f.updated_at) IS NOT NULL AND m.id IS NULL
ON DUPLICATE KEY UPDATE source_key=VALUES(source_key);
COMMIT;
