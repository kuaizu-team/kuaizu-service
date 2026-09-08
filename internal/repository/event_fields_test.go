package repository

import (
	"context"
	"database/sql/driver"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"strings"
	"testing"
	"time"
)

func TestProjectEventQueryScansAllFields(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewEventRepository(sqlx.NewDb(db, "capture_user_repo"))
	now := time.Now()
	columns := strings.Split("project_id,id,name,is_ranking,registration_deadline,article_url,organizer_name,description,official_website,participation_note,resource_url,qq_group,allow_cross_school,allow_cross_major,cross_school_major_rule,participation_mode,team_min_members,team_max_members,view_count,display_order,created_at,updated_at", ",")
	setCapturedQuery(columns, [][]driver.Value{{int64(5), int64(42), "event", int64(0), nil, nil, nil, nil, "https://example.com", "说明", nil, nil, int64(1), int64(1), "allow_cross_school_and_major", "both", int64(1), int64(3), int64(0), int64(0), now, now}})
	got, err := repo.ListByProjectIDs(context.Background(), []int{5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got[5]) != 1 {
		t.Fatal(got)
	}
	event := got[5][0]
	if *event.OfficialWebsite != "https://example.com" || *event.ParticipationNote != "说明" || *event.ParticipationMode != "both" || *event.TeamMinMembers != 1 || *event.TeamMaxMembers != 3 {
		t.Fatalf("lost fields: %#v", event)
	}
}

func TestEventUpdateBindsWebsiteAndNoteIncludingNull(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewEventRepository(sqlx.NewDb(db, "capture_user_repo"))
	website, note := "https://example.com", "保留说明"
	for _, createdAt := range []time.Time{{}, time.Now()} {
		for _, withValues := range []bool{true, false} {
			event := &models.Event{ID: 42, Name: "event", CreatedAt: createdAt}
			if withValues {
				event.OfficialWebsite = &website
				event.ParticipationNote = &note
			}
			if err := repo.Update(context.Background(), event); err != nil {
				t.Fatal(err)
			}
			capturedExec.Lock()
			query := normalizeSQL(capturedExec.query)
			args := append([]driver.NamedValue(nil), capturedExec.args...)
			capturedExec.Unlock()
			if !strings.Contains(query, "official_website = ?") || !strings.Contains(query, "participation_note = ?") || strings.Contains(query, "THEN participation_note ELSE NULL") {
				t.Fatal(query)
			}
			var gotWebsite, gotNote interface{}
			// SET assignments are emitted in order; both timestamps variants share these positions.
			gotWebsite = args[8].Value
			gotNote = args[14].Value
			if withValues && (gotWebsite != website || gotNote != note) {
				t.Fatalf("unexpected bound values: %#v", args)
			}
			if !withValues && (gotWebsite != nil || gotNote != nil) {
				t.Fatal("explicit NULL not bound")
			}
		}
	}
}

func TestReplacingCustomTimelinePreservesStoredOverallDeadline(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewEventRepository(sqlx.NewDb(db, "capture_user_repo"))
	if err := repo.ReplaceTimelineNodes(context.Background(), 42, nil); err != nil {
		t.Fatal(err)
	}
	capturedExec.Lock()
	query := normalizeSQL(capturedExec.query)
	capturedExec.Unlock()
	if !strings.Contains(query, "TRIM(title) NOT IN ('报名截止', '报名截止时间')") {
		t.Fatal(query)
	}
}

func eventAssociationFixture(parentID int64) ([]string, [][]driver.Value) {
	now := time.Now()
	columns := strings.Split("parent_id,id,name,is_ranking,registration_deadline,article_url,organizer_name,description,official_website,participation_note,resource_url,qq_group,allow_cross_school,allow_cross_major,cross_school_major_rule,participation_mode,team_min_members,team_max_members,view_count,display_order,created_at,updated_at", ",")
	rows := [][]driver.Value{{parentID, int64(42), "event", int64(0), nil, nil, nil, nil, "https://example.com", "说明", nil, nil, int64(1), int64(1), "allow_cross_school_and_major", "both", int64(1), int64(3), int64(0), int64(0), now, now}}
	return columns, rows
}

func assertCompleteAssociatedEvent(t *testing.T, event models.Event) {
	t.Helper()
	if event.OfficialWebsite == nil || *event.OfficialWebsite != "https://example.com" ||
		event.ParticipationNote == nil || *event.ParticipationNote != "说明" ||
		event.CrossSchoolMajorRule == nil || *event.CrossSchoolMajorRule != "allow_cross_school_and_major" ||
		event.ParticipationMode == nil || *event.ParticipationMode != "both" ||
		event.TeamMinMembers == nil || *event.TeamMinMembers != 1 ||
		event.TeamMaxMembers == nil || *event.TeamMaxMembers != 3 {
		t.Fatalf("associated event lost fields: %#v", event)
	}
}

func TestInformationEventQueryScansAllFields(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewInformationContentRepository(sqlx.NewDb(db, "capture_user_repo"))
	columns, rows := eventAssociationFixture(7)
	setCapturedQuery(columns, rows)
	items := []models.InformationContent{{ID: 7}}
	if err := repo.enrichEventsBatch(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	if len(items[0].Events) != 1 {
		t.Fatalf("events = %#v", items[0].Events)
	}
	assertCompleteAssociatedEvent(t, items[0].Events[0])
}

func TestRecommendationEventQueryScansAllFields(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewRecommendationRepository(sqlx.NewDb(db, "capture_user_repo"))
	columns, rows := eventAssociationFixture(5)
	setCapturedQuery(columns, rows)
	got, err := repo.listEventsByProjectIDs(context.Background(), []int{5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got[5]) != 1 {
		t.Fatalf("events = %#v", got)
	}
	assertCompleteAssociatedEvent(t, got[5][0])
}
