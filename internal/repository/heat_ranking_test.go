package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestProjectSchoolPriorityUsesHeatUpgradeAndStableRandomOrder(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewProjectRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQueryQueue(
		captureQueryResult{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}},
		captureQueryResult{columns: []string{"id"}},
	)
	sortBy := "school_priority"
	schoolID := 42
	seed := "7:2026-06-27"

	_, _, err := repo.List(context.Background(), ListParams{
		Page: 1, Size: 10, SortBy: &sortBy, UserSchoolID: &schoolID, RandomSeed: seed,
	})
	if err != nil {
		t.Fatal(err)
	}

	queries, args := capturedQueriesAndArgs()
	if len(queries) != 2 {
		t.Fatalf("query count = %d, want 2", len(queries))
	}
	query := normalizeSQL(queries[1])
	for _, want := range []string{
		"FROM project_like", "FROM project_favorite",
		"GREATEST(1,", "FLOOR((COALESCE(plc.like_count, 0) + COALESCE(pfc.favorite_count, 0) * 2) / 10)",
		"CRC32(CONCAT(?, ':', p.id)) ASC, p.id ASC",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q: %s", want, query)
		}
	}
	if len(args) != 4 || args[0].Value != int64(schoolID) || args[1].Value != seed || args[2].Value != int64(10) || args[3].Value != int64(0) {
		t.Fatalf("unexpected list args: %#v", args)
	}
}

func TestTalentSchoolPriorityKeepsAuthFirstThenUsesHeatUpgrade(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewTalentProfileRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQueryQueue(
		captureQueryResult{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}},
		captureQueryResult{columns: []string{"id"}},
	)
	sortBy := "school_priority"
	schoolID := 42
	seed := "7:2026-06-27"

	_, _, err := repo.List(context.Background(), TalentProfileListParams{
		Page: 1, Size: 10, SortBy: &sortBy, UserSchoolID: &schoolID, RandomSeed: seed,
	})
	if err != nil {
		t.Fatal(err)
	}

	queries, args := capturedQueriesAndArgs()
	if len(queries) != 2 {
		t.Fatalf("query count = %d, want 2", len(queries))
	}
	query := normalizeSQL(queries[1])
	authPos := strings.Index(query, "CASE WHEN u.auth_status = 1 THEN 0 ELSE 1 END ASC")
	heatPos := strings.Index(query, "GREATEST(1,")
	if authPos < 0 || heatPos < 0 || authPos > heatPos {
		t.Fatalf("certification must precede heat-adjusted tier: %s", query)
	}
	for _, want := range []string{
		"FROM talent_like", "FROM talent_favorite",
		"FLOOR((COALESCE(tlc.like_count, 0) + COALESCE(tfc.favorite_count, 0) * 2) / 10)",
		"CRC32(CONCAT(?, ':', tp.id)) ASC, tp.id ASC",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q: %s", want, query)
		}
	}
	if len(args) != 4 || args[0].Value != int64(schoolID) || args[1].Value != seed || args[2].Value != int64(10) || args[3].Value != int64(0) {
		t.Fatalf("unexpected list args: %#v", args)
	}
}

func capturedQueriesAndArgs() ([]string, []driver.NamedValue) {
	capturedQuery.Lock()
	defer capturedQuery.Unlock()
	return append([]string(nil), capturedQuery.queries...), append([]driver.NamedValue(nil), capturedQuery.args...)
}
