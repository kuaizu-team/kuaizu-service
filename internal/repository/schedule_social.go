package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

var ErrScheduleRule = errors.New("日程操作限制")
var ErrScheduleAccess = errors.New("日程不可用或没有访问权限")

const scheduleSocialColumns = `id,title,DATE_FORMAT(schedule_date,'%Y-%m-%d') AS date,all_day,TIME_FORMAT(schedule_time,'%H:%i') AS time,DATE_FORMAT(end_date,'%Y-%m-%d') AS end_date,TIME_FORMAT(end_time,'%H:%i') AS end_time,completed,location,notes,joint_id`

func (r *ScheduleRepository) Privacy(ctx context.Context, uid int) (models.SchedulePrivacy, error) {
	var p models.SchedulePrivacy
	err := r.db.GetContext(ctx, &p, `SELECT team_details,member_contacts FROM schedule_privacy WHERE user_id=?`, uid)
	if err == sql.ErrNoRows {
		err = nil
	}
	return p, err
}
func (r *ScheduleRepository) SavePrivacy(ctx context.Context, uid int, p models.SchedulePrivacy) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO schedule_privacy(user_id,team_details,member_contacts) VALUES(?,?,?) ON DUPLICATE KEY UPDATE team_details=VALUES(team_details),member_contacts=VALUES(member_contacts)`, uid, p.TeamDetails, p.MemberContacts)
	return err
}
func (r *ScheduleRepository) Share(ctx context.Context, uid int, kind, date string, id int64) (models.ScheduleShare, error) {
	share := models.ScheduleShare{OwnerID: uid, Kind: kind, Date: date, Payload: "{}"}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return share, err
	}
	defer tx.Rollback()
	if err = scheduleLockUsers(ctx, tx, []int{uid}); err != nil {
		return share, err
	}
	var count int
	if err = tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM schedule_share WHERE owner_id=? AND revoked=0 AND expires_at>NOW()`, uid); err != nil {
		return share, err
	}
	if count >= 100 {
		return share, fmt.Errorf("%w: 最多保留100条有效分享，请先撤销旧链接", ErrScheduleRule)
	}
	if kind == "item" {
		var item models.Schedule
		if err := tx.GetContext(ctx, &item, `SELECT `+scheduleSocialColumns+` FROM user_schedule WHERE id=? AND user_id=?`, id, uid); err != nil {
			return share, ErrScheduleAccess
		}
		// Share an independent snapshot, never membership or contact access.
		item.JointID = nil
		item.CreatedBy = nil
		item.ParticipantCount = 0
		item.Completed = false
		item.ID = 0
		payload, _ := json.Marshal(item)
		share.Payload = string(payload)
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return share, err
	}
	share.Token = hex.EncodeToString(bytes)
	res, err := tx.ExecContext(ctx, `INSERT INTO schedule_share(token,owner_id,kind,share_date,payload,expires_at) VALUES(?,?,?,?,?,DATE_ADD(NOW(),INTERVAL 7 DAY))`, share.Token, uid, kind, date, share.Payload)
	if err == nil {
		share.ID, err = res.LastInsertId()
	}
	if err != nil {
		return share, err
	}
	return share, tx.Commit()
}
func (r *ScheduleRepository) Shares(ctx context.Context, uid int) ([]models.ScheduleShare, error) {
	items := []models.ScheduleShare{}
	err := r.db.SelectContext(ctx, &items, `SELECT id,token,owner_id,kind,share_date,payload FROM schedule_share WHERE owner_id=? AND revoked=0 AND expires_at>NOW() ORDER BY id DESC LIMIT 100`, uid)
	return items, err
}
func (r *ScheduleRepository) Revoke(ctx context.Context, uid int, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE schedule_share SET revoked=1 WHERE id=? AND owner_id=?`, id, uid)
	return err
}
func scheduleShare(ctx context.Context, db sqlx.ExtContext, token string, lock bool) (models.ScheduleShare, error) {
	var s models.ScheduleShare
	query := `SELECT id,token,owner_id,kind,share_date,payload FROM schedule_share WHERE token=? AND revoked=0 AND expires_at>NOW()`
	if lock {
		query += ` FOR UPDATE`
	}
	err := sqlx.GetContext(ctx, db, &s, query, token)
	if err == sql.ErrNoRows {
		err = ErrScheduleAccess
	}
	return s, err
}
func (r *ScheduleRepository) ViewShare(ctx context.Context, token, date string) (models.ScheduleShare, []models.Schedule, error) {
	share, err := scheduleShare(ctx, r.db, token, false)
	if err != nil {
		return share, nil, err
	}
	items := []models.Schedule{}
	if share.Kind == "item" {
		var item models.Schedule
		err = json.Unmarshal([]byte(share.Payload), &item)
		items = append(items, item)
	} else {
		if share.Kind == "day" {
			date = share.Date
		}
		items, err = r.List(ctx, share.OwnerID, date, date)
		if err == nil && len(items) > 2000 {
			return share, nil, fmt.Errorf("%w: 当天日程过多，暂无法分享查看", ErrScheduleRule)
		}
		// Do not expose participants, completion or internal IDs through a calendar link.
		for i := range items {
			if items[i].JointID != nil {
				items[i].Title = "共同日程（忙碌）"
				items[i].Location = ""
				items[i].Notes = ""
			}
			items[i].ID = 0
			items[i].JointID = nil
			items[i].CreatedBy = nil
			items[i].ParticipantCount = 0
			items[i].Completed = false
		}
	}
	return share, items, err
}

type ScheduleTeam struct {
	ID   int    `db:"id" json:"id"`
	Name string `db:"name" json:"name"`
}

func (r *ScheduleRepository) Teams(ctx context.Context, uid int) ([]ScheduleTeam, error) {
	items := []ScheduleTeam{}
	err := r.db.SelectContext(ctx, &items, `SELECT p.id,p.name FROM project p WHERE p.deleted_at IS NULL AND p.status<>4 AND (p.creator_id=? OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=p.id AND m.user_id=?)) ORDER BY p.id DESC LIMIT 100`, uid, uid)
	return items, err
}
func scheduleTeamAllowed(ctx context.Context, db sqlx.ExtContext, pid, uid int) error {
	var exists bool
	err := sqlx.GetContext(ctx, db, &exists, `SELECT EXISTS(SELECT 1 FROM project p WHERE p.id=? AND p.deleted_at IS NULL AND p.status<>4 AND (p.creator_id=? OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=p.id AND m.user_id=?)))`, pid, uid, uid)
	if err == nil && !exists {
		return ErrScheduleAccess
	}
	return err
}
func (r *ScheduleRepository) TeamPeople(ctx context.Context, uid, pid int) ([]models.SchedulePerson, error) {
	if err := scheduleTeamAllowed(ctx, r.db, pid, uid); err != nil {
		return nil, err
	}
	people := []models.SchedulePerson{}
	err := r.db.SelectContext(ctx, &people, "SELECT u.id,COALESCE(u.nickname,'快组儿') AS name,0 AS profile_id FROM `user` u WHERE u.user_status<>1 AND (u.id=(SELECT creator_id FROM project WHERE id=?) OR EXISTS(SELECT 1 FROM project_members m WHERE m.project_id=? AND m.user_id=u.id)) ORDER BY (u.id=?) DESC,u.id LIMIT 200", pid, pid, uid)
	return people, err
}

type ScheduleBusy struct {
	UserID  int               `json:"userId"`
	Items   []models.Schedule `json:"items"`
	Details bool              `json:"details"`
}

func (r *ScheduleRepository) TeamBusy(ctx context.Context, uid, pid int, ids []int, date string) ([]ScheduleBusy, error) {
	if err := scheduleTeamAllowed(ctx, r.db, pid, uid); err != nil {
		return nil, err
	}
	result := []ScheduleBusy{}
	for _, id := range ids {
		if err := scheduleTeamAllowed(ctx, r.db, pid, id); err != nil {
			return nil, err
		}
		p, err := r.Privacy(ctx, id)
		if err != nil {
			return nil, err
		}
		items, err := r.List(ctx, id, date, date)
		if err != nil {
			return nil, err
		}
		if len(items) > 2000 {
			return nil, fmt.Errorf("%w: 日程过多，暂无法计算忙闲", ErrScheduleRule)
		}
		details := id == uid || p.TeamDetails
		for i := range items {
			items[i].ID = 0
			items[i].JointID = nil
			items[i].CreatedBy = nil
			items[i].ParticipantCount = 0
			items[i].Completed = false
			if !details {
				items[i].Title = "忙碌"
				items[i].Location = ""
				items[i].Notes = ""
			}
		}
		result = append(result, ScheduleBusy{id, items, details})
	}
	return result, nil
}

// All participant rows are locked in a consistent order. Concurrent creates see
// a current schedule set, and conflict acknowledgement is checked inside commit.
func scheduleLockUsers(ctx context.Context, tx *sqlx.Tx, ids []int) error {
	sort.Ints(ids)
	for _, id := range ids {
		var found int
		err := tx.GetContext(ctx, &found, "SELECT id FROM `user` WHERE id=? AND user_status<>1 FOR UPDATE", id)
		if err != nil {
			return ErrScheduleAccess
		}
	}
	return nil
}
func scheduleConflicts(ctx context.Context, tx *sqlx.Tx, ids []int, item models.Schedule) ([]int, error) {
	conflicts := []int{}
	_, end := models.ScheduleWindow(item)
	for _, id := range ids {
		existing := []models.Schedule{}
		err := tx.SelectContext(ctx, &existing, `SELECT `+scheduleSocialColumns+` FROM user_schedule WHERE user_id=? AND schedule_date<=? AND (schedule_date>=? OR end_date>=?) LIMIT 2001`, id, end.Format("2006-01-02"), item.Date, item.Date)
		if err != nil {
			return nil, err
		}
		if len(existing) > 2000 {
			return nil, fmt.Errorf("%w: 日程过多，请整理后再添加", ErrScheduleRule)
		}
		for _, old := range existing {
			if models.SchedulesOverlap(item, old) {
				conflicts = append(conflicts, id)
				break
			}
		}
	}
	return conflicts, nil
}
func scheduleInsert(ctx context.Context, tx *sqlx.Tx, uid int, item models.Schedule, joint *int64) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO user_schedule(user_id,title,schedule_date,all_day,schedule_time,end_date,end_time,location,notes,joint_id) VALUES(?,?,?,?,?,?,?,?,?,?)`, uid, item.Title, item.Date, item.AllDay, item.Time, item.EndDate, item.EndTime, item.Location, item.Notes, joint)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

type ScheduleAddResult struct {
	ID                int64 `json:"id"`
	NeedsConfirmation bool  `json:"needsConfirmation"`
	Conflicts         []int `json:"conflicts"`
	Existing          bool  `json:"existing"`
}

func (r *ScheduleRepository) CopyShare(ctx context.Context, uid int, token string, allow bool) (ScheduleAddResult, error) {
	result := ScheduleAddResult{Conflicts: []int{}}
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err = scheduleLockUsers(ctx, tx, []int{uid}); err != nil {
		return result, err
	}
	share, err := scheduleShare(ctx, tx, token, true)
	if err != nil {
		return result, err
	}
	if share.Kind != "item" {
		return result, ErrScheduleAccess
	}
	var previous int64
	err = tx.GetContext(ctx, &previous, `SELECT schedule_id FROM schedule_share_copy WHERE share_id=? AND user_id=?`, share.ID, uid)
	if err == nil {
		var count int
		if err = tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM user_schedule WHERE id=? AND user_id=?`, previous, uid); err != nil {
			return result, err
		}
		if count > 0 {
			result.ID = previous
			result.Existing = true
			return result, nil
		}
	}
	if err != nil && err != sql.ErrNoRows {
		return result, err
	}
	var item models.Schedule
	if err = json.Unmarshal([]byte(share.Payload), &item); err != nil {
		return result, err
	}
	if err = item.Validate(); err != nil {
		return result, err
	}
	result.Conflicts, err = scheduleConflicts(ctx, tx, []int{uid}, item)
	if err != nil {
		return result, err
	}
	if len(result.Conflicts) > 0 && !allow {
		result.NeedsConfirmation = true
		return result, nil
	}
	result.ID, err = scheduleInsert(ctx, tx, uid, item, nil)
	if err != nil {
		return result, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO schedule_share_copy(share_id,user_id,schedule_id) VALUES(?,?,?) ON DUPLICATE KEY UPDATE schedule_id=VALUES(schedule_id)`, share.ID, uid, result.ID)
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}
func (r *ScheduleRepository) CreateJoint(ctx context.Context, uid, pid int, token, key string, ids []int, item models.Schedule, allow bool) (ScheduleAddResult, error) {
	result := ScheduleAddResult{Conflicts: []int{}}
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	// Personal calendar links authorize exactly the owner + sender. Clients never choose arbitrary recipients.
	if token != "" {
		share, e := scheduleShare(ctx, tx, token, false)
		if e != nil {
			return result, e
		}
		if share.Kind == "item" || share.OwnerID == uid {
			return result, ErrScheduleAccess
		}
		if share.Kind == "day" && item.Date != share.Date {
			return result, fmt.Errorf("%w: 请选择分享当天的日期", ErrScheduleRule)
		}
		ids = []int{uid, share.OwnerID}
	} else {
		if err = scheduleTeamAllowed(ctx, tx, pid, uid); err != nil {
			return result, err
		}
		found := false
		unique := map[int]bool{}
		for _, id := range ids {
			if unique[id] {
				return result, fmt.Errorf("%w: 参与人重复", ErrScheduleRule)
			}
			unique[id] = true
			if id == uid {
				found = true
			}
			if err = scheduleTeamAllowed(ctx, tx, pid, id); err != nil {
				return result, err
			}
		}
		if !found {
			return result, fmt.Errorf("%w: 创建者须参与日程", ErrScheduleRule)
		}
	}
	if len(ids) < 2 || len(ids) > 20 {
		return result, fmt.Errorf("%w: 共同日程需为 2—20 人", ErrScheduleRule)
	}
	if err = scheduleLockUsers(ctx, tx, ids); err != nil {
		return result, err
	}
	// Lock the share again before writing so revocation cannot race with booking.
	if token != "" {
		if _, err = scheduleShare(ctx, tx, token, true); err != nil {
			return result, err
		}
	}
	err = tx.GetContext(ctx, &result.ID, `SELECT id FROM schedule_joint WHERE creator_id=? AND request_key=?`, uid, key)
	if err == nil {
		result.Existing = true
		return result, nil
	}
	if err != sql.ErrNoRows {
		return result, err
	}
	var count int
	if err = tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM schedule_joint WHERE creator_id=? AND created_at>=DATE_SUB(NOW(),INTERVAL 1 DAY)`, uid); err != nil {
		return result, err
	}
	if count >= 50 {
		return result, fmt.Errorf("%w: 今日共同日程创建次数已达上限", ErrScheduleRule)
	}
	result.Conflicts, err = scheduleConflicts(ctx, tx, ids, item)
	if err != nil {
		return result, err
	}
	if len(result.Conflicts) > 0 && !allow {
		result.NeedsConfirmation = true
		return result, nil
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO schedule_joint(creator_id,request_key) VALUES(?,?)`, uid, key)
	if err != nil {
		return result, err
	}
	result.ID, err = res.LastInsertId()
	if err != nil {
		return result, err
	}
	for _, id := range ids {
		if _, err = scheduleInsert(ctx, tx, id, item, &result.ID); err != nil {
			return result, err
		}
		if id != uid {
			if _, err = tx.ExecContext(ctx, `INSERT INTO schedule_notice(user_id,joint_id,message) VALUES(?,?,?)`, id, result.ID, "你被加入共同日程："+item.Title); err != nil {
				return result, err
			}
		}
	}
	return result, tx.Commit()
}
func (r *ScheduleRepository) JointPeople(ctx context.Context, uid int, jid int64) ([]models.SchedulePerson, error) {
	var found int
	err := r.db.GetContext(ctx, &found, `SELECT user_id FROM user_schedule WHERE joint_id=? AND user_id=?`, jid, uid)
	if err != nil {
		return nil, ErrScheduleAccess
	}
	people := []models.SchedulePerson{}
	err = r.db.SelectContext(ctx, &people, "SELECT u.id,COALESCE(u.nickname,'快组儿') AS name,COALESCE((SELECT id FROM talent_profile WHERE user_id=u.id LIMIT 1),0) AS profile_id,CASE WHEN p.member_contacts=1 THEN COALESCE(u.phone,'') ELSE '' END AS phone,CASE WHEN p.member_contacts=1 THEN COALESCE(u.wechat_id,'') ELSE '' END AS wechat,CASE WHEN p.member_contacts=1 THEN COALESCE(u.email,'') ELSE '' END AS email FROM user_schedule s JOIN `user` u ON u.id=s.user_id LEFT JOIN schedule_privacy p ON p.user_id=u.id WHERE s.joint_id=? ORDER BY u.id", jid)
	return people, err
}

// Creator cancellation removes all copies atomically; another participant only leaves their own copy.
func (r *ScheduleRepository) LeaveJoint(ctx context.Context, uid int, jid int64) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var creator int
	if err = tx.GetContext(ctx, &creator, `SELECT creator_id FROM schedule_joint WHERE id=? FOR UPDATE`, jid); err != nil {
		return ErrScheduleAccess
	}
	var own int64
	if err = tx.GetContext(ctx, &own, `SELECT id FROM user_schedule WHERE joint_id=? AND user_id=?`, jid, uid); err != nil {
		return ErrScheduleAccess
	}
	people := []int{}
	if err = tx.SelectContext(ctx, &people, `SELECT user_id FROM user_schedule WHERE joint_id=? ORDER BY user_id`, jid); err != nil {
		return err
	}
	if creator == uid {
		_, err = tx.ExecContext(ctx, `DELETE FROM user_schedule WHERE joint_id=?`, jid)
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM user_schedule WHERE joint_id=? AND user_id=?`, jid, uid)
	}
	if err != nil {
		return err
	}
	message := "共同日程有成员退出"
	if creator == uid {
		message = "创建者已取消共同日程"
	}
	for _, id := range people {
		if id != uid {
			if _, err = tx.ExecContext(ctx, `INSERT INTO schedule_notice(user_id,joint_id,message) VALUES(?,?,?)`, id, jid, message); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
func (r *ScheduleRepository) Notices(ctx context.Context, uid int) ([]models.ScheduleNotice, error) {
	notices := []models.ScheduleNotice{}
	err := r.db.SelectContext(ctx, &notices, `SELECT id,joint_id,message,is_read FROM schedule_notice WHERE user_id=? ORDER BY id DESC LIMIT 100`, uid)
	return notices, err
}
func (r *ScheduleRepository) ReadNotices(ctx context.Context, uid int, through int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE schedule_notice SET is_read=1 WHERE user_id=? AND id<=?`, uid, through)
	return err
}
