package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestCooperationAdminScopeMatrix(t *testing.T) {
	for _, tc := range []struct {
		role    int
		schools []int
		school  int
		edit    bool
		allowed bool
	}{
		{1, nil, 88, true, true}, {2, []int{42}, 42, true, true}, {2, []int{}, 42, true, false},
		{2, []int{42}, 88, false, false}, {3, []int{42}, 42, false, true}, {3, []int{42}, 42, true, false}, {4, []int{42}, 42, false, false},
	} {
		t.Run(fmt.Sprintf("%d/%d/%v", tc.role, tc.school, tc.edit), func(t *testing.T) {
			if (CooperationAdminScope{Role: tc.role, SchoolIDs: tc.schools}).CanAccess(tc.school, tc.edit) != tc.allowed {
				t.Fatal("scope boundary wrong")
			}
		})
	}
}
func TestCooperationApplicationListRestrictsUserAndRemovesPrivateReview(t *testing.T) {
	raw, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	r := NewCampusCooperationRepository(sqlx.NewDb(raw, "sqlmock"))
	m.ExpectQuery(`(?s)SELECT COUNT.*WHERE a.user_id=\?`).WithArgs(99).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	m.ExpectQuery(`(?s)SELECT a.id.*WHERE a.user_id=\? ORDER BY a.applied_at DESC,a.id DESC LIMIT \? OFFSET \?`).WithArgs(99, 20, 0).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "profile_snapshot", "review_remark", "reviewed_by"}).AddRow(12, 99, `{"phone":"private"}`, "internal-only", 7))
	rows, _, err := r.ListApplications(context.Background(), 0, 99, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(rows)
	if strings.Contains(string(payload), "private") || strings.Contains(string(payload), "internal-only") || rows[0].ReviewedBy != nil {
		t.Fatalf("private data leaked: %s", payload)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCooperationQueueOptInRanksAllTypesWithoutReadingSuppressedArchives(t *testing.T) {
	raw, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	r := NewStatusNotificationRepository(sqlx.NewDb(raw, "sqlmock"))
	m.ExpectQuery(`(?s)SELECT sn.id.*LEFT JOIN campus_cooperation_application.*WHERE sn.user_id=\?.*ORDER BY sn.priority DESC,sn.created_at DESC,sn.id DESC LIMIT 1`).WithArgs(99).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "type", "priority", "message_id", "cooperation_id", "cooperation_application_id", "cooperation_title", "applied_at"}).AddRow(25, 99, "cooperation-approved", 200, 30, 9, 12, "合作标题", time.Now()))
	m.ExpectExec(regexp.QuoteMeta(`UPDATE status_notification SET displayed_at=CURRENT_TIMESTAMP WHERE user_id=? AND displayed_at IS NULL AND id<>?`)).WithArgs(99, int64(25)).WillReturnResult(sqlmock.NewResult(0, 2))
	n, err := r.GetPendingIncludingCooperation(context.Background(), 99)
	if err != nil || n.CooperationID == nil || *n.CooperationID != 9 || n.Priority != 200 {
		t.Fatalf("%+v %v", n, err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCooperationJSONNullArraysAreNormalized(t *testing.T) {
	null := "null"
	c := models.CampusCooperation{LinksJSON: &null, ImagesJSON: &null, ContactsJSON: &null}
	if err := c.DecodeContent(); err != nil {
		t.Fatal(err)
	}
	if c.Links == nil || c.Images == nil || c.Contacts == nil {
		t.Fatal("contract arrays became null")
	}
}

func TestLegacyMessageListExcludesCooperationButOptInIncludesIt(t *testing.T) {
	for _, include := range []bool{false, true} {
		t.Run(fmt.Sprint(include), func(t *testing.T) {
			raw, m, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()
			r := NewStatusNotificationRepository(sqlx.NewDb(raw, "sqlmock"))
			where := "user_id=?"
			if !include {
				where += " AND type NOT IN ('cooperation-approved','cooperation-rejected')"
			}
			m.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM user_status_message WHERE ` + where)).WithArgs(99).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
			m.ExpectQuery(regexp.QuoteMeta(`SELECT `+messageColumns+` FROM user_status_message WHERE `+where+` ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`)).WithArgs(99, 20, 0).WillReturnRows(sqlmock.NewRows([]string{"id", "type", "payload", "read_at", "fullscreen_displayed", "fullscreen_displayed_at", "created_at"}))
			if include {
				_, _, err = r.ListMessages(context.Background(), 99, 1, 20, nil)
			} else {
				_, _, err = r.ListLegacyMessages(context.Background(), 99, 1, 20, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
