package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestProjectViewLogNotifyProgressUsesDistinctUsersAndThirtyDayRepeatWindow(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewProjectViewLogRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQuery([]string{"distinct_user_count", "is_new_user"}, [][]driver.Value{{int64(3), true}})

	got, err := repo.NotifyProgress(context.Background(), 10, 20, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.DistinctUserCount != 3 || !got.IsNewUser {
		t.Fatalf("unexpected progress: %#v", got)
	}
	capturedQuery.Lock()
	query := normalizeSQL(capturedQuery.query)
	args := append([]driver.NamedValue(nil), capturedQuery.args...)
	capturedQuery.Unlock()

	for _, want := range []string{
		"COUNT(DISTINCT CASE WHEN user_id IS NOT NULL AND user_id<>? THEN user_id END)",
		"NOT EXISTS",
		"prev.duration_ms IS NULL",
		"prev.id <",
		"DATE_SUB(NOW(), INTERVAL 30 DAY)",
		"DATE_SUB((",
		"INTERVAL 30 DAY",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("project notify progress query missing %q: %s", want, query)
		}
	}
	if len(args) != 8 || args[0].Value != int64(100) || args[2].Value != int64(20) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestTalentViewLogNotifyProgressUsesDistinctUsersAndThirtyDayRepeatWindow(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewTalentViewLogRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQuery([]string{"distinct_user_count", "is_new_user"}, [][]driver.Value{{int64(6), true}})

	got, err := repo.NotifyProgress(context.Background(), 10, 20, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.DistinctUserCount != 6 || !got.IsNewUser {
		t.Fatalf("unexpected progress: %#v", got)
	}
	capturedQuery.Lock()
	query := normalizeSQL(capturedQuery.query)
	args := append([]driver.NamedValue(nil), capturedQuery.args...)
	capturedQuery.Unlock()

	for _, want := range []string{
		"COUNT(DISTINCT CASE WHEN user_id IS NOT NULL AND user_id<>? THEN user_id END)",
		"FROM talent_view_log prev",
		"prev.duration_ms IS NULL",
		"prev.id <",
		"DATE_SUB(NOW(), INTERVAL 30 DAY)",
		"INTERVAL 30 DAY",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("talent notify progress query missing %q: %s", want, query)
		}
	}
	if len(args) != 8 || args[0].Value != int64(100) || args[2].Value != int64(20) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestProjectViewLogCountUnreadVisitsDeduplicatesOwnerAndRecentRepeats(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewProjectViewLogRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQuery([]string{"count"}, [][]driver.Value{{int64(2)}})

	got, err := repo.CountUnreadVisits(context.Background(), 10, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
	capturedQuery.Lock()
	query := normalizeSQL(capturedQuery.query)
	args := append([]driver.NamedValue(nil), capturedQuery.args...)
	capturedQuery.Unlock()

	for _, want := range []string{
		"COUNT(DISTINCT vl.user_id)",
		"vl.user_id IS NOT NULL",
		"vl.user_id <> ?",
		"interaction_type = 'visit'",
		"NOT EXISTS",
		"prev.duration_ms IS NULL",
		"DATE_SUB(vl.viewed_at, INTERVAL 30 DAY)",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("project unread visits query missing %q: %s", want, query)
		}
	}
	if len(args) != 4 || args[0].Value != int64(10) || args[1].Value != int64(100) || args[2].Value != int64(100) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestTalentViewLogCountUnreadVisitsDeduplicatesOwnerAndRecentRepeats(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewTalentViewLogRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQuery([]string{"count"}, [][]driver.Value{{int64(2)}})

	got, err := repo.CountUnreadVisits(context.Background(), 10, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
	capturedQuery.Lock()
	query := normalizeSQL(capturedQuery.query)
	args := append([]driver.NamedValue(nil), capturedQuery.args...)
	capturedQuery.Unlock()

	for _, want := range []string{
		"COUNT(DISTINCT vl.user_id)",
		"vl.user_id IS NOT NULL",
		"vl.user_id <> ?",
		"interaction_type = 'visit'",
		"NOT EXISTS",
		"prev.duration_ms IS NULL",
		"DATE_SUB(vl.viewed_at, INTERVAL 30 DAY)",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("talent unread visits query missing %q: %s", want, query)
		}
	}
	if len(args) != 4 || args[0].Value != int64(10) || args[1].Value != int64(100) || args[2].Value != int64(100) {
		t.Fatalf("unexpected args: %#v", args)
	}
}
