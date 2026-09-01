package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestTalentKeywordSearchUsesDegradedMatchingAcrossDisplayedFields(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewTalentProfileRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQueryQueue(
		captureQueryResult{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}},
		captureQueryResult{columns: []string{"id"}},
	)

	keyword := "  计算机%_  "
	_, _, err := repo.List(context.Background(), TalentProfileListParams{
		Page: 1, Size: 10, Keyword: &keyword,
	})
	if err != nil {
		t.Fatal(err)
	}

	queries, args := capturedQueriesAndArgs()
	if len(queries) != 2 {
		t.Fatalf("query count = %d, want 2", len(queries))
	}
	for _, query := range queries {
		normalized := normalizeSQL(query)
		for _, want := range []string{
			"WHEN u.nickname IS NULL OR TRIM(u.nickname) = '' OR TRIM(u.nickname) = '匿名用户'",
			"THEN '快组儿' ELSE TRIM(u.nickname) END",
			"SELECT search_school.school_name FROM school search_school",
			"SELECT search_major.major_name FROM major search_major",
			"CONCAT(CAST(u.grade AS CHAR), '级')",
			"CAST(tp.skill_summary AS CHAR)",
			"tp.self_evaluation",
			"tp.project_experience",
		} {
			if !strings.Contains(normalized, want) {
				t.Fatalf("query missing %q: %s", want, normalized)
			}
		}
	}

	listQuery := normalizeSQL(queries[1])
	for _, want := range []string{"CASE WHEN", "THEN 4", "THEN 3", "THEN 2", "THEN 1", "END DESC, tp.updated_at DESC"} {
		if !strings.Contains(listQuery, want) {
			t.Fatalf("ranking query missing %q: %s", want, listQuery)
		}
	}
	patterns := map[interface{}]bool{}
	for _, arg := range args {
		patterns[arg.Value] = true
	}
	for _, want := range []string{"%计算机!%!_%", "%计算%", "%算机%", "%计%", "%!%%", "%!_%"} {
		if !patterns[want] {
			t.Fatalf("missing search pattern %q in args", want)
		}
	}
}

func TestTalentKeywordLikePatternEscapesWildcards(t *testing.T) {
	if got, want := talentKeywordLikePattern("A!_%"), "%A!!!_!%%"; got != want {
		t.Fatalf("talentKeywordLikePattern() = %q, want %q", got, want)
	}
}
