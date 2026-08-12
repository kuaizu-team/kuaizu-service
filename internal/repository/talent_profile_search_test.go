package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

func TestTalentKeywordSearchUsesDisplayedNicknameAndProfileFields(t *testing.T) {
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
			"THEN ? ELSE TRIM(u.nickname) END LIKE ? ESCAPE '!'",
			"search_school.school_name LIKE ? ESCAPE '!'",
			"search_major.major_name LIKE ? ESCAPE '!'",
			"CAST(tp.skill_summary AS CHAR) LIKE ? ESCAPE '!'",
			"tp.self_evaluation LIKE ? ESCAPE '!'",
			"tp.project_experience LIKE ? ESCAPE '!'",
		} {
			if !strings.Contains(normalized, want) {
				t.Fatalf("query missing %q: %s", want, normalized)
			}
		}
	}

	const whereArgCount = 7
	if len(args) != whereArgCount*2+2 {
		t.Fatalf("arg count = %d, want %d", len(args), whereArgCount*2+2)
	}
	wantPattern := "%计算机!%!_%"
	for _, offset := range []int{0, whereArgCount} {
		if args[offset].Value != models.DefaultUserNickname {
			t.Fatalf("default nickname arg = %#v, want %q", args[offset].Value, models.DefaultUserNickname)
		}
		for i := offset + 1; i < offset+whereArgCount; i++ {
			if args[i].Value != wantPattern {
				t.Fatalf("pattern arg %d = %#v, want %q", i, args[i].Value, wantPattern)
			}
		}
	}
}

func TestTalentKeywordLikePatternEscapesWildcards(t *testing.T) {
	if got, want := talentKeywordLikePattern("A!_%"), "%A!!!_!%%"; got != want {
		t.Fatalf("talentKeywordLikePattern() = %q, want %q", got, want)
	}
}
