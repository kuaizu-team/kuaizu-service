package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestProjectKeywordSearchUsesSchoolAndTagExistsForCountAndList(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewProjectRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQueryQueue(
		captureQueryResult{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}},
		captureQueryResult{columns: []string{"id"}},
	)
	keyword := "比赛"

	projects, total, err := repo.List(context.Background(), ListParams{Page: 1, Size: 10, Keyword: &keyword})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(projects) != 0 {
		t.Fatalf("unexpected result: total=%d projects=%#v", total, projects)
	}

	capturedQuery.Lock()
	queries := append([]string(nil), capturedQuery.queries...)
	args := append([]driver.NamedValue(nil), capturedQuery.args...)
	capturedQuery.Unlock()
	if len(queries) != 2 {
		t.Fatalf("query count = %d, want 2", len(queries))
	}
	for _, query := range queries {
		normalized := normalizeSQL(query)
		for _, want := range []string{"EXISTS (SELECT 1 FROM school ks", "EXISTS (SELECT 1 FROM project_tag_relation ptr", "pt.status=1"} {
			if !strings.Contains(normalized, want) {
				t.Fatalf("query missing %q: %s", want, normalized)
			}
		}
	}
	if len(args) != 6 {
		t.Fatalf("list args = %#v", args)
	}
	for i := 0; i < 4; i++ {
		if args[i].Value != "%比赛%" {
			t.Fatalf("keyword arg %d = %#v", i, args[i].Value)
		}
	}
}
