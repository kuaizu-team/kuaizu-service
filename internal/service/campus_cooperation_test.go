package service

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

func cooperationTestService(t *testing.T) (*CampusCooperationService, sqlmock.Sqlmock) {
	t.Helper()
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	db := sqlx.NewDb(raw, "sqlmock")
	repo := repository.New(db)
	return NewCampusCooperationService(repo, nil, nil), mock
}
func expectCooperationAdmin(mock sqlmock.Sqlmock, role int, owned []int) {
	var school interface{}
	if role == 3 {
		school = 42
	}
	mock.ExpectQuery(`SELECT role,school_id,status FROM admin_user WHERE id=\? FOR UPDATE`).WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"role", "school_id", "status"}).AddRow(role, school, 1))
	if role == 2 {
		rows := sqlmock.NewRows([]string{"school_id"})
		for _, id := range owned {
			rows.AddRow(id)
		}
		mock.ExpectQuery(`SELECT school_id FROM admin_school_relation WHERE admin_user_id=\? AND is_owner=1 ORDER BY school_id FOR UPDATE`).WithArgs(7).WillReturnRows(rows)
	}
}
func cooperationRows(schoolID int) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "commercial_order_id", "school_id", "title", "introduction", "company_name", "school_name", "application_required", "published"}).AddRow(9, 3, schoolID, "校园体验计划", "这是计划", "公司X", "学校", true, true)
}
func expectCooperation(mock sqlmock.Sqlmock, schoolID int) {
	mock.ExpectQuery(`(?s)SELECT c.id.*WHERE c.id=\? FOR UPDATE`).WithArgs(int64(9)).WillReturnRows(cooperationRows(schoolID))
}

type cooperationJSONArgs struct{ forbidden []string }

func (c cooperationJSONArgs) Match(v driver.Value) bool {
	text, ok := v.(string)
	if !ok || !json.Valid([]byte(text)) {
		return false
	}
	for _, key := range c.forbidden {
		if strings.Contains(text, key) {
			return false
		}
	}
	return true
}

func TestCooperationPublicProjectionHidesOnlyContactsUntilApproved(t *testing.T) {
	c := &models.CampusCooperation{ID: 9, LinksVisible: true, ContactsVisible: true, ApplicationRequired: true,
		Links: []models.CooperationLink{{Title: "详情", URL: "https://example.org"}}, Contacts: []models.CooperationContact{{Type: "wechat", Value: "private-contact"}}}
	for _, status := range []string{"", "pending", "rejected", "approved"} {
		view := cooperationPublic(c, status)
		bytes, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		if len(view.Links) != 1 {
			t.Fatal("application mode incorrectly hid public links")
		}
		if strings.Contains(string(bytes), "private-contact") != (status == "approved") {
			t.Fatalf("contact leak/unlock error for %s: %s", status, bytes)
		}
		if view.CanApply != (status == "" || status == "rejected") {
			t.Fatalf("canApply incorrect for %s", status)
		}
	}
	c.ApplicationRequired = false
	if len(cooperationPublic(c, "").Contacts) != 1 {
		t.Fatal("normal mode lost public contacts")
	}
	c.ContactsVisible = false
	if len(cooperationPublic(c, "approved").Contacts) != 0 {
		t.Fatal("approved applicant bypassed contact visibility switch")
	}
}
func TestCooperationContentValidation(t *testing.T) {
	for _, tc := range []struct {
		name         string
		title        string
		linksVisible bool
		link         models.CooperationLink
		bad          bool
	}{
		{"twenty unicode characters", strings.Repeat("校", 20), true, models.CooperationLink{Title: "链接", URL: "https://example.org"}, false},
		{"long title", strings.Repeat("校", 21), false, models.CooperationLink{}, true},
		{"missing visible address", "标题", true, models.CooperationLink{Title: "链接"}, true},
		{"restricted incomplete link", "标题", false, models.CooperationLink{}, true},
		{"script URL", "标题", true, models.CooperationLink{Title: "链接", URL: "javascript:alert(1)"}, true},
		{"credential URL", "标题", true, models.CooperationLink{Title: "链接", URL: "https://name:secret@example.org"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.link.Public = &tc.linksVisible
			in := CooperationContentInput{Title: tc.title, Introduction: "计划", LinksVisible: tc.linksVisible, Links: []models.CooperationLink{tc.link}}
			if (in.Validate() != nil) != tc.bad {
				t.Fatalf("wrong validation for %+v", in)
			}
		})
	}
}

func TestCooperationClaimRejectsNonOwnerRolesAndRetainedFinancialAccess(t *testing.T) {
	for _, role := range []int{1, 2, 3, 4} {
		t.Run(fmt.Sprint(role), func(t *testing.T) {
			s, m := cooperationTestService(t)
			m.ExpectBegin()
			expectCooperationAdmin(m, role, nil)
			m.ExpectRollback()
			if _, err := s.Claim(context.Background(), 7, 42, 3); err == nil {
				t.Fatal("non-owner claim succeeded")
			}
			if err := m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestCooperationClaimIsIdempotentEvenAfterOrderClosed(t *testing.T) {
	s, m := cooperationTestService(t)
	m.ExpectBegin()
	expectCooperationAdmin(m, 2, []int{42})
	m.ExpectQuery(`SELECT status FROM commercial_order WHERE id=\? FOR UPDATE`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("closed"))
	m.ExpectQuery(`SELECT id FROM campus_cooperation WHERE commercial_order_id=\? AND school_id=\?`).WithArgs(int64(3), 42).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	m.ExpectQuery(`(?s)SELECT c.id.*WHERE c.id=\?`).WithArgs(int64(9)).WillReturnRows(cooperationRows(42))
	m.ExpectCommit()
	c, err := s.Claim(context.Background(), 7, 42, 3)
	if err != nil || c.ID != 9 {
		t.Fatalf("%+v %v", c, err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCooperationReviewAllAuthorizedRolesAndAtomicNotification(t *testing.T) {
	for _, role := range []int{1, 2, 3} {
		for _, status := range []string{"approved", "rejected"} {
			t.Run(fmt.Sprintf("role%d-%s", role, status), func(t *testing.T) {
				s, m := cooperationTestService(t)
				s.message = NewMessageService(s.repo, nil)
				dispatched := int64(0)
				s.dispatchSubscribe = func(id int64) { dispatched = id }
				m.ExpectBegin()
				expectCooperationAdmin(m, role, []int{42})
				m.ExpectQuery(`SELECT cooperation_id FROM campus_cooperation_application WHERE id=\?`).WithArgs(int64(12)).WillReturnRows(sqlmock.NewRows([]string{"cooperation_id"}).AddRow(9))
				expectCooperation(m, 42)
				m.ExpectQuery(`SELECT id,cooperation_id,user_id,attempt_no,status,applied_at FROM campus_cooperation_application WHERE id=\? FOR UPDATE`).WithArgs(int64(12)).WillReturnRows(sqlmock.NewRows([]string{"id", "cooperation_id", "user_id", "attempt_no", "status", "applied_at"}).AddRow(12, 9, 99, 1, "pending", time.Now()))
				m.ExpectExec(`UPDATE campus_cooperation_application SET status=.*AND status='pending'`).WithArgs(status, 7, sqlmock.AnyArg(), nil, int64(12)).WillReturnResult(sqlmock.NewResult(0, 1))
				kind, priority := "cooperation-rejected", 10
				if status == "approved" {
					kind, priority = "cooperation-approved", 200
				}
				m.ExpectExec(`INSERT INTO status_notification`).WithArgs(99, kind, int64(12), priority).WillReturnResult(sqlmock.NewResult(25, 1))
				m.ExpectExec(`INSERT INTO user_status_message`).WithArgs(99, kind, "cooperation-review:12", int64(25), cooperationJSONArgs{[]string{"contacts", "phone", "email", "wechat", "openid"}}).WillReturnResult(sqlmock.NewResult(30, 1))
				if status == "approved" {
					m.ExpectExec(`(?s)INSERT INTO wx_subscribe_delivery`).WithArgs(99, models.MsgBizKeyCampusCooperationResult, sqlmock.AnyArg(), "pages/campus-cooperation-detail/campus-cooperation-detail?id=9", models.WxSubscribeDeliveryPending).WillReturnResult(sqlmock.NewResult(40, 1))
				}
				m.ExpectCommit()
				if err := s.Review(context.Background(), 7, 12, status, nil); err != nil {
					t.Fatal(err)
				}
				if (dispatched > 0) != (status == "approved") {
					t.Fatalf("wrong dispatch %d", dispatched)
				}
				if err := m.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
func TestCooperationReviewArchiveFailureRollsBackDecision(t *testing.T) {
	s, m := cooperationTestService(t)
	m.ExpectBegin()
	expectCooperationAdmin(m, 3, nil)
	m.ExpectQuery(`SELECT cooperation_id`).WithArgs(int64(12)).WillReturnRows(sqlmock.NewRows([]string{"cooperation_id"}).AddRow(9))
	expectCooperation(m, 42)
	m.ExpectQuery(`SELECT id,cooperation_id,user_id,attempt_no,status,applied_at`).WithArgs(int64(12)).WillReturnRows(sqlmock.NewRows([]string{"id", "cooperation_id", "user_id", "status", "applied_at"}).AddRow(12, 9, 99, "pending", time.Now()))
	m.ExpectExec(`UPDATE campus_cooperation_application`).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec(`INSERT INTO status_notification`).WillReturnResult(sqlmock.NewResult(25, 1))
	m.ExpectExec(`INSERT INTO user_status_message`).WillReturnError(errors.New("archive unavailable"))
	m.ExpectRollback()
	if err := s.Review(context.Background(), 7, 12, "approved", nil); err == nil {
		t.Fatal("archive failure committed review")
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCooperationReviewDeniesOtherSchool(t *testing.T) {
	s, m := cooperationTestService(t)
	m.ExpectBegin()
	expectCooperationAdmin(m, 3, nil)
	m.ExpectQuery(`SELECT cooperation_id`).WithArgs(int64(12)).WillReturnRows(sqlmock.NewRows([]string{"cooperation_id"}).AddRow(9))
	expectCooperation(m, 88)
	m.ExpectRollback()
	if err := s.Review(context.Background(), 7, 12, "approved", nil); err == nil {
		t.Fatal("cross-school review succeeded")
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCooperationReviewReplayDoesNotCreateMessages(t *testing.T) {
	s, m := cooperationTestService(t)
	m.ExpectBegin()
	expectCooperationAdmin(m, 1, nil)
	m.ExpectQuery(`SELECT cooperation_id`).WillReturnRows(sqlmock.NewRows([]string{"cooperation_id"}).AddRow(9))
	expectCooperation(m, 42)
	m.ExpectQuery(`SELECT id,cooperation_id,user_id,attempt_no,status,applied_at`).WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(12, "approved"))
	m.ExpectCommit()
	if err := s.Review(context.Background(), 7, 12, "approved", nil); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCooperationApplySchoolBoundaryAndRejectedReapplication(t *testing.T) {
	for _, scenario := range []string{"other-school", "no-school", "pending", "approved", "rejected", "first"} {
		t.Run(scenario, func(t *testing.T) {
			s, m := cooperationTestService(t)
			m.ExpectBegin()
			expectCooperation(m, 42)
			var school interface{} = 42
			if scenario == "other-school" {
				school = 88
			}
			if scenario == "no-school" {
				school = nil
			}
			m.ExpectQuery("SELECT school_id,user_status FROM `user` WHERE id=\\? FOR UPDATE").WithArgs(99).WillReturnRows(sqlmock.NewRows([]string{"school_id", "user_status"}).AddRow(school, 0))
			if scenario == "other-school" || scenario == "no-school" {
				m.ExpectRollback()
				if _, err := s.Apply(context.Background(), 99, 9); err == nil {
					t.Fatal("invalid school accepted")
				}
				if err := m.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
				return
			}
			m.ExpectQuery(`SELECT EXISTS`).WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			rows := sqlmock.NewRows([]string{"id", "cooperation_id", "user_id", "attempt_no", "status"})
			if scenario != "first" {
				rows.AddRow(12, 9, 99, 1, scenario)
			}
			m.ExpectQuery(`(?s)SELECT id,cooperation_id.*ORDER BY attempt_no DESC LIMIT 1 FOR UPDATE`).WithArgs(int64(9), 99).WillReturnRows(rows)
			expectedAttempt := 1
			expectedID := int64(12)
			if scenario == "first" || scenario == "rejected" {
				if scenario == "rejected" {
					expectedAttempt = 2
				}
				expectedID = 13
				// No subscription state is consulted: refusal/failure cannot block applying.
				m.ExpectQuery(`(?s)SELECT JSON_OBJECT.*FROM .*user.*WHERE u.id=\?`).WithArgs(99).WillReturnRows(sqlmock.NewRows([]string{"snapshot"}).AddRow(`{"userId":99,"nickname":"当前名片","schoolId":42}`))
				m.ExpectExec(`INSERT INTO campus_cooperation_application`).WithArgs(int64(9), 99, expectedAttempt, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(13, 1))
				m.ExpectQuery(`SELECT id,cooperation_id,user_id,attempt_no,status,applied_at`).WithArgs(int64(13)).WillReturnRows(sqlmock.NewRows([]string{"id", "cooperation_id", "user_id", "attempt_no", "status", "applied_at"}).AddRow(13, 9, 99, expectedAttempt, "pending", time.Now()))
			}
			m.ExpectCommit()
			a, err := s.Apply(context.Background(), 99, 9)
			if err != nil || a.ID != expectedID || a.AttemptNo != expectedAttempt {
				t.Fatalf("%+v %v", a, err)
			}
			if err := m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestCooperationSaveContentRejectsMemberAndForeignImage(t *testing.T) {
	for _, role := range []int{2, 3} {
		t.Run(fmt.Sprint(role), func(t *testing.T) {
			s, m := cooperationTestService(t)
			m.ExpectBegin()
			expectCooperationAdmin(m, role, []int{42})
			expectCooperation(m, 42)
			if role == 2 {
				m.ExpectQuery(`SELECT object_key FROM campus_cooperation_image WHERE object_key=\? AND cooperation_id=\? FOR UPDATE`).WithArgs("campus-cooperation-images/foreign.jpg", int64(9)).WillReturnRows(sqlmock.NewRows([]string{"object_key"}))
			}
			m.ExpectRollback()
			in := CooperationContentInput{Title: "标题", Introduction: "计划", Images: []string{"campus-cooperation-images/foreign.jpg"}}
			if err := s.SaveContent(context.Background(), 7, 9, in); err == nil {
				t.Fatal("member edit/foreign image accepted")
			}
			if err := m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCooperationReviewOutboxFailureRollsBackAndDoesNotDispatch(t *testing.T) {
	s, m := cooperationTestService(t)
	s.message = NewMessageService(s.repo, nil)
	s.dispatchSubscribe = func(int64) { t.Fatal("dispatched before successful commit") }
	m.ExpectBegin()
	expectCooperationAdmin(m, 1, nil)
	m.ExpectQuery(`SELECT cooperation_id`).WillReturnRows(sqlmock.NewRows([]string{"cooperation_id"}).AddRow(9))
	expectCooperation(m, 42)
	m.ExpectQuery(`SELECT id,cooperation_id,user_id,attempt_no,status,applied_at`).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "status", "applied_at"}).AddRow(12, 99, "pending", time.Now()))
	m.ExpectExec(`UPDATE campus_cooperation_application`).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec(`INSERT INTO status_notification`).WillReturnResult(sqlmock.NewResult(25, 1))
	m.ExpectExec(`INSERT INTO user_status_message`).WillReturnResult(sqlmock.NewResult(30, 1))
	m.ExpectExec(`(?s)INSERT INTO wx_subscribe_delivery`).WillReturnError(errors.New("outbox unavailable"))
	m.ExpectRollback()
	if err := s.Review(context.Background(), 7, 12, "approved", nil); err == nil {
		t.Fatal("outbox failure committed review")
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCooperationReviewConflictingReplayCannotOverwriteDecision(t *testing.T) {
	s, m := cooperationTestService(t)
	m.ExpectBegin()
	expectCooperationAdmin(m, 1, nil)
	m.ExpectQuery(`SELECT cooperation_id`).WillReturnRows(sqlmock.NewRows([]string{"cooperation_id"}).AddRow(9))
	expectCooperation(m, 42)
	m.ExpectQuery(`SELECT id,cooperation_id,user_id,attempt_no,status,applied_at`).WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(12, "rejected"))
	m.ExpectRollback()
	if err := s.Review(context.Background(), 7, 12, "approved", nil); err == nil {
		t.Fatal("overwrote completed review")
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCooperationSameOrderCanBeClaimedByDifferentSchools(t *testing.T) {
	for _, schoolID := range []int{42, 88} {
		t.Run(fmt.Sprint(schoolID), func(t *testing.T) {
			s, m := cooperationTestService(t)
			m.ExpectBegin()
			expectCooperationAdmin(m, 2, []int{schoolID})
			m.ExpectQuery(`SELECT status FROM commercial_order WHERE id=\? FOR UPDATE`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("available"))
			m.ExpectQuery(`SELECT id FROM campus_cooperation WHERE commercial_order_id=\? AND school_id=\?`).WithArgs(int64(3), schoolID).WillReturnRows(sqlmock.NewRows([]string{"id"}))
			m.ExpectExec(`INSERT INTO campus_cooperation`).WithArgs(int64(3), schoolID, 7).WillReturnResult(sqlmock.NewResult(9, 1))
			m.ExpectQuery(`(?s)SELECT c.id.*WHERE c.id=\?`).WithArgs(int64(9)).WillReturnRows(cooperationRows(schoolID))
			m.ExpectCommit()
			c, err := s.Claim(context.Background(), 7, schoolID, 3)
			if err != nil || c.SchoolID != schoolID {
				t.Fatalf("%+v %v", c, err)
			}
			if err := m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCooperationLinksCanAllBeRemoved(t *testing.T) {
	in := CooperationContentInput{Title: "标题", Introduction: "计划", LinksVisible: true}
	if err := in.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCooperationPerLinkVisibility(t *testing.T) {
	public, restricted := true, false
	for _, legacyPublic := range []bool{false, true} {
		for _, applicationRequired := range []bool{false, true} {
			for _, status := range []string{"", "pending", "rejected", "approved", "unknown"} {
				t.Run(fmt.Sprintf("legacy=%v/application=%v/status=%s", legacyPublic, applicationRequired, status), func(t *testing.T) {
					c := &models.CampusCooperation{ApplicationRequired: applicationRequired, LinksVisible: legacyPublic,
						Links: []models.CooperationLink{
							{Title: "公开介绍", URL: "https://example.org/public", Public: &public},
							{Title: "仅通过者可见", URL: "https://example.org/approval-only", Public: &restricted},
							{Title: "旧链接", URL: "https://example.org/legacy"},
						}}
					view := cooperationPublic(c, status)
					want := 1
					unlocked := !applicationRequired || status == "approved"
					if unlocked {
						want++
					}
					if legacyPublic || unlocked {
						want++
					}
					if len(view.Links) != want || !view.LinksVisible {
						t.Fatalf("wrong visible links: %+v", view)
					}
					body, err := json.Marshal(view)
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(string(body), "approval-only") != unlocked || strings.Contains(string(body), "仅通过者可见") != unlocked {
						t.Fatalf("restricted title or URL leaked/was lost: %s", body)
					}
					// Projection must not mutate the stored links.
					if len(c.Links) != 3 || c.Links[2].Public != nil {
						t.Fatal("projection mutated source")
					}
					c.Links = c.Links[1:2]
					view = cooperationPublic(c, status)
					if view.LinksVisible != unlocked || (len(view.Links) == 1) != unlocked {
						t.Fatalf("wrong approval-only projection: %+v", view)
					}
				})
			}
		}
	}
}

func TestCooperationSaveRejectsOldEditorWithoutPerLinkPermission(t *testing.T) {
	in := CooperationContentInput{Title: "标题", Introduction: "计划", LinksVisible: true,
		Links: []models.CooperationLink{{Title: "旧编辑页", URL: "https://example.org"}}}
	if err := in.Validate(); err == nil || !strings.Contains(err.Error(), "刷新后台") {
		t.Fatalf("missing permissions accepted: %v", err)
	}
}

func TestCooperationSavePersistsIndependentLinkPermissions(t *testing.T) {
	s, m := cooperationTestService(t)
	m.ExpectBegin()
	expectCooperationAdmin(m, 2, []int{42})
	expectCooperation(m, 42)
	public, restricted := true, false
	in := CooperationContentInput{Title: "标题", Introduction: "计划", ApplicationRequired: true, Published: true,
		Links: []models.CooperationLink{
			{Title: "公开介绍", URL: "https://example.org/public", Public: &public},
			{Title: "专属入口", URL: "https://example.org/private", Public: &restricted},
		}}
	links, _ := json.Marshal(in.Links)
	m.ExpectExec("UPDATE campus_cooperation SET").
		WithArgs("标题", "计划", false, string(links), false, nil, "null", true, "null", true, true, 7, int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	if err := s.SaveContent(context.Background(), 7, 9, in); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
	stored := string(links)
	c := &models.CampusCooperation{LinksJSON: &stored, ApplicationRequired: true}
	if err := c.DecodeContent(); err != nil {
		t.Fatal(err)
	}
	if len(cooperationPublic(c, "pending").Links) != 1 || len(cooperationPublic(c, "approved").Links) != 2 {
		t.Fatal("saved visibility did not round-trip")
	}
}

func TestCooperationTagsReuseProjectLimits(t *testing.T) {
	for _, tags := range [][]string{{"科研", "AI写作"}, {}, {" 重复 ", "重复"}, {"这是一个超过十二个字符的合作标签"}, {"1", "2", "3", "4", "5", "6"}} {
		in := CooperationContentInput{Title: "标题", Introduction: "计划", Tags: &tags}
		err := in.Validate()
		wantError := len(tags) > 5 || (len(tags) > 0 && (tags[0] == "重复" || utf8.RuneCountInString(tags[0]) > 12))
		if (err != nil) != wantError {
			t.Fatalf("tags=%+v err=%v", tags, err)
		}
	}
}

func TestCooperationTagsSaveInSameAuthorizedTransaction(t *testing.T) {
	for _, failLibrary := range []bool{false, true} {
		t.Run(fmt.Sprint(failLibrary), func(t *testing.T) {
			s, m := cooperationTestService(t)
			m.ExpectBegin()
			expectCooperationAdmin(m, 2, []int{42})
			expectCooperation(m, 42)
			m.ExpectExec("UPDATE campus_cooperation SET title=").WillReturnResult(sqlmock.NewResult(0, 1))
			first := m.ExpectExec("(?s)INSERT INTO project_tag.*ON DUPLICATE KEY UPDATE").WithArgs("科研")
			if failLibrary {
				first.WillReturnError(errors.New("library write failed"))
				m.ExpectRollback()
			} else {
				first.WillReturnResult(sqlmock.NewResult(28, 1))
				m.ExpectExec("(?s)INSERT INTO project_tag.*ON DUPLICATE KEY UPDATE").WithArgs("AI写作").WillReturnResult(sqlmock.NewResult(29, 1))
				m.ExpectExec(regexp.QuoteMeta("UPDATE campus_cooperation SET tags=? WHERE id=?")).WithArgs(`["科研","AI写作"]`, int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
				m.ExpectCommit()
			}
			tags := []string{" 科研 ", "AI写作"}
			err := s.SaveContent(context.Background(), 7, 9, CooperationContentInput{Title: "标题", Introduction: "计划", Tags: &tags})
			if (err != nil) != failLibrary {
				t.Fatalf("error=%v", err)
			}
			if err := m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCooperationExplicitEmptyTagsClearWithoutCreatingLibraryEntries(t *testing.T) {
	s, m := cooperationTestService(t)
	m.ExpectBegin()
	expectCooperationAdmin(m, 2, []int{42})
	expectCooperation(m, 42)
	m.ExpectExec("UPDATE campus_cooperation SET title=").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec(regexp.QuoteMeta("UPDATE campus_cooperation SET tags=? WHERE id=?")).WithArgs("[]", int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	tags := []string{}
	if err := s.SaveContent(context.Background(), 7, 9, CooperationContentInput{Title: "标题", Introduction: "计划", Tags: &tags}); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
