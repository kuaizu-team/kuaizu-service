package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

const messageColumns = `id,type,payload,read_at,fullscreen_displayed,fullscreen_displayed_at,created_at`

// Snapshots deliberately have no foreign key to their source: removing a
// project/application/invitation must not remove its user's archived message.
const statusMessageSnapshot = `SELECT sn.user_id, sn.type, CONCAT('status:',sn.id) AS source_key, sn.id AS status_notification_id,
	JSON_OBJECT('projectId',p.id,'projectName',p.name,'nickname',u.nickname,
	'projectCreatedAt',p.created_at,'appliedAt',COALESCE(pa.applied_at,ob.created_at,pmr.joined_at),
	'discussingAt',COALESCE(pa.discussing_at,ob.discussing_at),
	'rejectedAt',COALESCE(pa.rejected_at,ob.rejected_at),'joinedAt',COALESCE(pa.joined_at,ob.admitted_at),
	'reviewerRoleName',COALESCE(rr.name,obr.name),'assignedRoleName',COALESCE(ar.name,oar.name,pmrr.name),
	'operatorName',sender.nickname,'removedAt',pmr.removed_at,'eventAt',sn.created_at) AS payload,
	sn.created_at
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
	LEFT JOIN ` + "`user`" + ` u ON u.id=sn.user_id
	LEFT JOIN ` + "`user`" + ` sender ON sender.id=ob.sender_id`

func archiveStatusNotificationTx(ctx context.Context, tx *sqlx.Tx, id int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO user_status_message
		(user_id,type,source_key,status_notification_id,payload,created_at,fullscreen_displayed)
		SELECT snapshot.*,0 FROM (`+statusMessageSnapshot+` WHERE sn.id=?) snapshot
		ON DUPLICATE KEY UPDATE source_key=VALUES(source_key)`, id)
	return err
}

// Discussing is archive-only; it never enters the existing priority queue.
func CreateDiscussingMessageTx(ctx context.Context, tx *sqlx.Tx, id int, olive bool) error {
	table, alias, user, project, applied, role, source := "project_application", "pa", "pa.user_id", "pa.project_id", "pa.applied_at", "pa.reviewer_role", "application"
	if olive {
		table, alias, user, project, applied, role, source = "olive_branch_record", "pa", "pa.receiver_id", "pa.related_project_id", "pa.created_at", "pa.operator_role", "olive"
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO user_status_message(user_id,type,source_key,payload,fullscreen_displayed,created_at)
		SELECT `+user+`,'application-discussing',CONCAT('discussing:`+source+`:',pa.id),
		JSON_OBJECT('projectId',p.id,'projectName',p.name,'nickname',u.nickname,'appliedAt',`+applied+`,
		'discussingAt',pa.discussing_at,'eventAt',pa.discussing_at,'reviewerRoleName',pr.name,'context',?),0,pa.discussing_at
		FROM `+table+` `+alias+` JOIN project p ON p.id=`+project+`
		LEFT JOIN project_role pr ON pr.code=`+role+` LEFT JOIN `+"`user`"+` u ON u.id=`+user+`
		WHERE pa.id=? ON DUPLICATE KEY UPDATE source_key=VALUES(source_key)`, source, id)
	return err
}

func ArchiveProjectStatusTx(ctx context.Context, tx *sqlx.Tx, id int, kind string, userIDs ...int) error {
	event, key := "p.created_at", "CONCAT('project-published:',p.id)"
	switch kind {
	case "recruit-completed":
		event, key = "p.recruit_completed_at", "CONCAT('recruit-completed:',p.id,':',UNIX_TIMESTAMP(p.recruit_completed_at))"
	case "project-ended":
		event, key = "p.ended_at", "CONCAT('project-ended:',p.id)"
	case "project-published":
	default:
		return fmt.Errorf("unsupported project message type")
	}
	userSQL := "p.creator_id"
	args := []interface{}{}
	if len(userIDs) > 0 {
		userSQL = "?"
		args = append(args, userIDs[0])
	}
	args = append(args, kind)
	if len(userIDs) > 0 {
		args = append(args, userIDs[0])
	}
	args = append(args, id)
	_, err := tx.ExecContext(ctx, `INSERT INTO user_status_message(user_id,type,source_key,payload,fullscreen_displayed,created_at)
		SELECT `+userSQL+`,?,`+key+`,JSON_OBJECT('projectId',p.id,'projectName',p.name,
		'nickname',u.nickname,'projectCreatedAt',p.created_at,'eventAt',`+event+`,
		'recruitCompletedAt',p.recruit_completed_at,'endedAt',p.ended_at,
		'totalViews',p.view_count,
		'likeCount',(SELECT COUNT(*) FROM project_like WHERE project_id=p.id),
		'favoriteCount',(SELECT COUNT(*) FROM project_favorite WHERE project_id=p.id),
		'shareCount',(SELECT COUNT(*) FROM project_share WHERE project_id=p.id)),0,`+event+`
		FROM project p LEFT JOIN `+"`user`"+` u ON u.id=`+userSQL+` WHERE p.id=?
		ON DUPLICATE KEY UPDATE source_key=VALUES(source_key)`, args...)
	return err
}

func archiveTalentPublishedTx(ctx context.Context, tx *sqlx.Tx, id int) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO user_status_message(user_id,type,source_key,payload,fullscreen_displayed,created_at)
		SELECT t.user_id,'talent-published',CONCAT('talent-published:',t.id,':',UNIX_TIMESTAMP(t.updated_at)),
		JSON_OBJECT('nickname',u.nickname,'eventAt',t.updated_at),0,t.updated_at
		FROM talent_profile t LEFT JOIN `+"`user`"+` u ON u.id=t.user_id WHERE t.id=?
		ON DUPLICATE KEY UPDATE source_key=VALUES(source_key)`, id)
	return err
}

func archiveSuperAdminInvitationTx(ctx context.Context, tx *sqlx.Tx, userID int) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO user_status_message(user_id,type,source_key,payload,fullscreen_displayed,created_at)
		SELECT pi.user_id,'super-admin-invitation',CONCAT('invitation:',pi.id,':',UNIX_TIMESTAMP(pi.expire_at)),
		JSON_OBJECT('nickname',u.nickname,'schoolName',s.school_name,'eventAt',pi.updated_at),0,pi.updated_at
		FROM pending_invitation pi LEFT JOIN `+"`user`"+` u ON u.id=pi.user_id
		LEFT JOIN school s ON s.id=u.school_id WHERE pi.user_id=?
		ON DUPLICATE KEY UPDATE source_key=VALUES(source_key)`, userID)
	return err
}

func decodeStatusMessage(m *models.StatusMessage) error {
	m.Params = map[string]interface{}{}
	if m.Payload != nil {
		if err := json.Unmarshal([]byte(*m.Payload), &m.Params); err != nil {
			return err
		}
	}
	return nil
}

func (r *StatusNotificationRepository) ListMessages(ctx context.Context, userID, page, size int, read *bool) ([]models.StatusMessage, int64, error) {
	where, args := "user_id=?", []interface{}{userID}
	if read != nil {
		if *read {
			where += " AND read_at IS NOT NULL"
		} else {
			where += " AND read_at IS NULL"
		}
	}
	var total int64
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM user_status_message WHERE `+where, args...); err != nil {
		return nil, 0, err
	}
	messages := []models.StatusMessage{}
	args = append(args, size, (page-1)*size)
	if err := r.db.SelectContext(ctx, &messages, `SELECT `+messageColumns+` FROM user_status_message WHERE `+where+` ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`, args...); err != nil {
		return nil, 0, err
	}
	for i := range messages {
		if err := decodeStatusMessage(&messages[i]); err != nil {
			return nil, 0, err
		}
	}
	return messages, total, nil
}

func (r *StatusNotificationRepository) GetMessage(ctx context.Context, userID int, id int64) (*models.StatusMessage, error) {
	var m models.StatusMessage
	if err := r.db.GetContext(ctx, &m, `SELECT `+messageColumns+` FROM user_status_message WHERE id=? AND user_id=?`, id, userID); err != nil {
		return nil, err
	}
	if err := decodeStatusMessage(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *StatusNotificationRepository) LatestMessage(ctx context.Context, userID int, kind string, projectID int) (*models.StatusMessage, error) {
	args := []interface{}{userID, kind}
	where := "user_id=? AND type=?"
	if projectID > 0 {
		where += " AND JSON_EXTRACT(payload,'$.projectId')=?"
		args = append(args, projectID)
	}
	var m models.StatusMessage
	if err := r.db.GetContext(ctx, &m, `SELECT `+messageColumns+` FROM user_status_message WHERE `+where+` ORDER BY created_at DESC,id DESC LIMIT 1`, args...); err != nil {
		return nil, err
	}
	if err := decodeStatusMessage(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *StatusNotificationRepository) UnreadMessageCount(ctx context.Context, userID int) (int64, error) {
	var count int64
	err := r.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM user_status_message WHERE user_id=? AND read_at IS NULL`, userID)
	return count, err
}

func (r *StatusNotificationRepository) MarkMessage(ctx context.Context, userID int, id int64, displayed, read bool) error {
	sets := []string{}
	if displayed {
		sets = append(sets, "fullscreen_displayed=1", "fullscreen_displayed_at=COALESCE(fullscreen_displayed_at,CURRENT_TIMESTAMP)")
	}
	if read {
		sets = append(sets, "read_at=COALESCE(read_at,CURRENT_TIMESTAMP)")
	}
	if len(sets) == 0 {
		return nil
	}
	// Check ownership separately so repeat reads/displays remain idempotent.
	var exists int
	if err := r.db.GetContext(ctx, &exists, `SELECT 1 FROM user_status_message WHERE id=? AND user_id=?`, id, userID); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE user_status_message SET `+strings.Join(sets, ",")+` WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	if result == nil {
		return sql.ErrNoRows
	}
	return nil
}
