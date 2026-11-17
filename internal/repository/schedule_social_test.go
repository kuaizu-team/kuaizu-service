package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"regexp"
	"testing"
)

func socialMock(t *testing.T) (*ScheduleRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewScheduleRepository(sqlx.NewDb(db, "sqlmock")), m
}
func socialShareRows(kind string, owner int, payload string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "token", "owner_id", "kind", "share_date", "payload"}).AddRow(1, "token", owner, kind, "", payload)
}
func socialItem() models.Schedule {
	clock, end, date := "09:00", "10:00", "2026-11-17"
	return models.Schedule{Title: "见面", Date: date, Time: &clock, EndDate: &date, EndTime: &end}
}
func socialLock(m sqlmock.Sqlmock, ids ...int) {
	for _, id := range ids {
		m.ExpectQuery("SELECT id FROM.*FOR UPDATE").WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
	}
}
func socialConflict(m sqlmock.Sqlmock, uid int, found bool) {
	rows := sqlmock.NewRows([]string{"id", "title", "date", "all_day", "time", "end_date", "end_time", "completed", "location", "notes", "joint_id"})
	if found {
		rows.AddRow(9, "原日程", "2026-11-17", false, "09:30", "2026-11-17", "10:30", false, "", "", nil)
	}
	m.ExpectQuery("SELECT .* FROM user_schedule WHERE user_id=.*LIMIT 2001").WithArgs(uid, "2026-11-17", "2026-11-17", "2026-11-17").WillReturnRows(rows)
}
func TestSharedCopyConflictNeverWritesWithoutAcknowledgement(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(fmt.Sprint(allow), func(t *testing.T) {
			r, m := socialMock(t)
			item := socialItem()
			payload, _ := json.Marshal(item)
			m.ExpectBegin()
			socialLock(m, 7)
			m.ExpectQuery("SELECT .* FROM schedule_share .*revoked=0.*expires_at>NOW.*FOR UPDATE").WithArgs("token").WillReturnRows(socialShareRows("item", 8, string(payload)))
			m.ExpectQuery("SELECT schedule_id FROM schedule_share_copy").WithArgs(int64(1), 7).WillReturnError(sql.ErrNoRows)
			socialConflict(m, 7, true)
			if allow {
				m.ExpectExec("INSERT INTO user_schedule").WillReturnResult(sqlmock.NewResult(12, 1))
				m.ExpectExec("INSERT INTO schedule_share_copy").WithArgs(int64(1), 7, int64(12)).WillReturnResult(sqlmock.NewResult(0, 1))
				m.ExpectCommit()
			} else {
				m.ExpectRollback()
			}
			result, err := r.CopyShare(context.Background(), 7, "token", allow)
			if err != nil {
				t.Fatal(err)
			}
			if result.NeedsConfirmation == allow || len(result.Conflicts) != 1 {
				t.Fatalf("%+v", result)
			}
			if err = m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestJointCreateAtomicNotificationsAndIdempotence(t *testing.T) {
	for _, mode := range []string{"success", "partial failure", "retry", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			r, m := socialMock(t)
			ctx := context.Background()
			m.ExpectBegin()
			m.ExpectQuery("SELECT .* FROM schedule_share .*revoked=0").WithArgs("token").WillReturnRows(socialShareRows("calendar", 8, "{}"))
			socialLock(m, 7, 8)
			m.ExpectQuery("SELECT .* FROM schedule_share .*FOR UPDATE").WithArgs("token").WillReturnRows(socialShareRows("calendar", 8, "{}"))
			lookup := m.ExpectQuery("SELECT id FROM schedule_joint WHERE creator_id=.*request_key=").WithArgs(7, "stable-request-key")
			if mode == "retry" {
				lookup.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
				m.ExpectRollback()
			} else {
				lookup.WillReturnError(sql.ErrNoRows)
				m.ExpectQuery("SELECT COUNT.* FROM schedule_joint").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
				socialConflict(m, 7, mode == "conflict")
				socialConflict(m, 8, false)
				if mode == "conflict" {
					m.ExpectRollback()
				} else {
					m.ExpectExec("INSERT INTO schedule_joint").WithArgs(7, "stable-request-key").WillReturnResult(sqlmock.NewResult(42, 1))
					m.ExpectExec("INSERT INTO user_schedule").WithArgs(7, "见面", "2026-11-17", false, "09:00", "2026-11-17", "10:00", "", "", int64(42)).WillReturnResult(sqlmock.NewResult(1, 1))
					second := m.ExpectExec("INSERT INTO user_schedule").WithArgs(8, "见面", "2026-11-17", false, "09:00", "2026-11-17", "10:00", "", "", int64(42))
					if mode == "partial failure" {
						second.WillReturnError(fmt.Errorf("disk error"))
						m.ExpectRollback()
					} else {
						second.WillReturnResult(sqlmock.NewResult(2, 1))
						m.ExpectExec("INSERT INTO schedule_notice").WithArgs(8, int64(42), "你被加入共同日程：见面").WillReturnResult(sqlmock.NewResult(1, 1))
						m.ExpectCommit()
					}
				}
			}
			result, err := r.CreateJoint(ctx, 7, 0, "token", "stable-request-key", []int{999}, socialItem(), false)
			if mode == "partial failure" {
				if err == nil {
					t.Fatal("partial commit")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if mode == "retry" && !result.Existing {
				t.Fatal("retry duplicated event")
			}
			if mode == "conflict" && !result.NeedsConfirmation {
				t.Fatal("conflict silently saved")
			}
			if err = m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestScheduleTeamAuthorizationAndBusyRedaction(t *testing.T) {
	r, m := socialMock(t)
	ctx := context.Background()
	allowed := func(uid int, yes bool) {
		m.ExpectQuery("SELECT EXISTS.*project p").WithArgs(3, uid, uid).WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(yes))
	}
	allowed(7, false)
	if _, err := r.TeamBusy(ctx, 7, 3, []int{8}, "2026-11-17"); err != ErrScheduleAccess {
		t.Fatal("outsider allowed")
	}
	allowed(7, true)
	allowed(8, false)
	if _, err := r.TeamBusy(ctx, 7, 3, []int{8}, "2026-11-17"); err != ErrScheduleAccess {
		t.Fatal("removed target allowed")
	}
	allowed(7, true)
	allowed(8, true)
	m.ExpectQuery("SELECT team_details,member_contacts").WithArgs(8).WillReturnError(sql.ErrNoRows)
	m.ExpectQuery("SELECT .* FROM user_schedule WHERE user_id=").WithArgs(8, "2026-11-17", "2026-11-17", "2026-11-17", "2026-11-17").WillReturnRows(sqlmock.NewRows([]string{"id", "title", "date", "all_day", "time", "location", "notes", "completed", "joint_id", "created_by", "participant_count"}).AddRow(9, "隐私标题", "2026-11-17", false, "09:00", "隐私地址", "隐私备注", true, 42, 8, 3))
	busy, err := r.TeamBusy(ctx, 7, 3, []int{8}, "2026-11-17")
	if err != nil {
		t.Fatal(err)
	}
	item := busy[0].Items[0]
	if busy[0].Details || item.Title != "忙碌" || item.Location != "" || item.Notes != "" || item.ID != 0 || item.JointID != nil || item.CreatedBy != nil || item.Completed {
		t.Fatalf("private data leaked: %+v", item)
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestJointContactsRequiresCurrentParticipation(t *testing.T) {
	r, m := socialMock(t)
	m.ExpectQuery(regexp.QuoteMeta(`SELECT user_id FROM user_schedule WHERE joint_id=? AND user_id=?`)).WithArgs(int64(42), 7).WillReturnError(sql.ErrNoRows)
	if _, err := r.JointPeople(context.Background(), 7, 42); err != ErrScheduleAccess {
		t.Fatal("contacts exposed to outsider")
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestShareRevokedOrExpiredIsDeniedBeforeCopy(t *testing.T) {
	r, m := socialMock(t)
	m.ExpectBegin()
	socialLock(m, 7)
	m.ExpectQuery("SELECT .* FROM schedule_share .*revoked=0.*expires_at>NOW.*FOR UPDATE").WithArgs("token").WillReturnError(sql.ErrNoRows)
	m.ExpectRollback()
	if _, err := r.CopyShare(context.Background(), 7, "token", true); err != ErrScheduleAccess {
		t.Fatal("expired link accepted")
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestJointLeaveAndCancelPreserveOtherSchedules(t *testing.T) {
	for _, creator := range []int{7, 8} {
		t.Run(fmt.Sprint(creator), func(t *testing.T) {
			r, m := socialMock(t)
			m.ExpectBegin()
			m.ExpectQuery("SELECT creator_id FROM schedule_joint .*FOR UPDATE").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"creator_id"}).AddRow(creator))
			m.ExpectQuery("SELECT id FROM user_schedule WHERE joint_id=.*user_id=").WithArgs(int64(42), 7).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(12))
			m.ExpectQuery("SELECT user_id FROM user_schedule WHERE joint_id=").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(7).AddRow(8))
			message := "共同日程有成员退出"
			if creator == 7 {
				m.ExpectExec(regexp.QuoteMeta(`DELETE FROM user_schedule WHERE joint_id=?`) + "$").WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 2))
				message = "创建者已取消共同日程"
			} else {
				m.ExpectExec(regexp.QuoteMeta(`DELETE FROM user_schedule WHERE joint_id=? AND user_id=?`)+"$").WithArgs(int64(42), 7).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			m.ExpectExec("INSERT INTO schedule_notice").WithArgs(8, int64(42), message).WillReturnResult(sqlmock.NewResult(0, 1))
			m.ExpectCommit()
			if err := r.LeaveJoint(context.Background(), 7, 42); err != nil {
				t.Fatal(err)
			}
			if err := m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestJointCopyCannotBeModifiedThroughPrivateEndpoint(t *testing.T) {
	r, m := socialMock(t)
	item := socialItem()
	item.ID = 12
	m.ExpectExec("UPDATE user_schedule SET title=.*WHERE id=.*AND user_id=.*AND joint_id IS NULL").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectQuery("SELECT completed FROM user_schedule WHERE id=.*user_id=.*joint_id IS NULL").WithArgs(int64(12), 7).WillReturnError(sql.ErrNoRows)
	if err := r.Update(context.Background(), 7, &item, nil); err != sql.ErrNoRows {
		t.Fatal("shared item editable through private PUT")
	}
	m.ExpectExec("DELETE FROM user_schedule WHERE id=.*user_id=.*joint_id IS NULL").WithArgs(int64(12), 7).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := r.Delete(context.Background(), 7, 12); err != sql.ErrNoRows {
		t.Fatal("shared item removable through private DELETE")
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

type scheduleSnapshotMatcher struct{}

func (scheduleSnapshotMatcher) Match(value driver.Value) bool {
	raw, ok := value.(string)
	if !ok {
		return false
	}
	var item models.Schedule
	return json.Unmarshal([]byte(raw), &item) == nil && item.ID == 0 && item.JointID == nil && item.CreatedBy == nil && item.ParticipantCount == 0 && !item.Completed && item.Title == "共同安排" && item.Notes == "已同意分享的备注"
}
func TestJointSnapshotShareDoesNotGrantParticipation(t *testing.T) {
	r, m := socialMock(t)
	m.ExpectBegin()
	socialLock(m, 7)
	m.ExpectQuery("SELECT COUNT.* FROM schedule_share WHERE owner_id=.*revoked=0").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	m.ExpectQuery("SELECT .* FROM user_schedule WHERE id=.*user_id=").WithArgs(int64(12), 7).WillReturnRows(sqlmock.NewRows([]string{"id", "title", "date", "all_day", "time", "end_date", "end_time", "completed", "location", "notes", "joint_id"}).AddRow(12, "共同安排", "2026-11-17", false, "09:00", "2026-11-17", "10:00", true, "教室", "已同意分享的备注", 42))
	m.ExpectExec("INSERT INTO schedule_share").WithArgs(sqlmock.AnyArg(), 7, "item", "", scheduleSnapshotMatcher{}).WillReturnResult(sqlmock.NewResult(1, 1))
	m.ExpectCommit()
	share, err := r.Share(context.Background(), 7, "item", "", 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(share.Token) != 64 {
		t.Fatal("weak token")
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestSharedCopyRetryUsesExistingCopy(t *testing.T) {
	r, m := socialMock(t)
	payload, _ := json.Marshal(socialItem())
	m.ExpectBegin()
	socialLock(m, 7)
	m.ExpectQuery("SELECT .* FROM schedule_share .*FOR UPDATE").WithArgs("token").WillReturnRows(socialShareRows("item", 8, string(payload)))
	m.ExpectQuery("SELECT schedule_id FROM schedule_share_copy").WithArgs(int64(1), 7).WillReturnRows(sqlmock.NewRows([]string{"schedule_id"}).AddRow(12))
	m.ExpectQuery("SELECT COUNT.* FROM user_schedule WHERE id=.*user_id=").WithArgs(int64(12), 7).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	m.ExpectRollback()
	result, err := r.CopyShare(context.Background(), 7, "token", true)
	if err != nil || !result.Existing || result.ID != 12 {
		t.Fatalf("%+v %v", result, err)
	}
	if err = m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
