package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestAdminListSchoolDirectoryScope(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewAdminUserRepository(sqlx.NewDb(db, "capture_user_repo"))
	keyword := "Alice"
	schoolID := 22
	viewerID := 31
	setCapturedQueryQueue(
		captureQueryResult{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}},
		captureQueryResult{columns: []string{"id"}},
	)

	if _, _, err := repo.List(context.Background(), AdminUserListParams{
		Page: 1, Size: 10, Keyword: &keyword,
		SchoolID: &schoolID, SchoolAdminScope: true, ViewerAdminID: &viewerID,
	}); err != nil {
		t.Fatal(err)
	}

	queries, args := capturedQueriesAndArgs()
	if len(queries) != 2 {
		t.Fatalf("query count = %d, want 2", len(queries))
	}
	query := normalizeSQL(queries[1])
	for _, want := range []string{
		"(au.nickname LIKE ? OR au.phone LIKE ?)",
		"au.role IN (?, ?) AND au.school_id = ?",
		"FROM admin_school_relation scope_rel",
		"scope_rel.school_id = ?",
		"scope_rel.commission_rate > 0",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("directory query missing %q: %s", want, query)
		}
	}
	if strings.Contains(query, "au.username LIKE") {
		t.Fatalf("directory keyword search exposed hidden usernames: %s", query)
	}
	if len(args) != 10 {
		t.Fatalf("list args = %d, want 10: %#v", len(args), args)
	}
}
