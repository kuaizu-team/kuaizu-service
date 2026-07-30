package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func captureEventListQuery(t *testing.T, params EventListParams) string {
	t.Helper()
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewEventRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQueryQueue(
		captureQueryResult{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}},
		captureQueryResult{columns: []string{"id"}},
	)

	if _, _, err := repo.List(context.Background(), params); err != nil {
		t.Fatal(err)
	}
	queries, _ := capturedQueriesAndArgs()
	if len(queries) != 2 {
		t.Fatalf("query count = %d, want 2", len(queries))
	}
	return normalizeSQL(queries[1])
}

func TestEventListKeepsPublicDefaultOrder(t *testing.T) {
	query := captureEventListQuery(t, EventListParams{Page: 1, Size: 10})
	want := "ORDER BY CASE WHEN e.registration_deadline IS NULL THEN 1 ELSE 0 END ASC, e.registration_deadline ASC, e.display_order DESC, e.created_at DESC, e.id DESC"
	if !strings.Contains(query, want) {
		t.Fatalf("public event order changed: %s", query)
	}
}

func TestEventListUsesExplicitAdminOrder(t *testing.T) {
	query := captureEventListQuery(t, EventListParams{
		Page: 1, Size: 10, SortBy: "displayOrder", Order: "asc",
	})
	want := "ORDER BY CASE WHEN e.display_order IS NULL THEN 1 ELSE 0 END ASC, e.display_order ASC, e.id DESC"
	if !strings.Contains(query, want) {
		t.Fatalf("explicit event order missing: %s", query)
	}
}
