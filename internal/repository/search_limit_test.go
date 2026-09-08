package repository

import (
	"context"
	"strings"
	"testing"
)

func TestSearchKeywordUnicodeBoundary(t *testing.T) {
	for _, unit := range []string{"a", "中", "🙂"} {
		allowed, denied := strings.Repeat(unit, MaxSearchKeywordRunes), strings.Repeat(unit, MaxSearchKeywordRunes+1)
		if ValidateSearchKeyword(&allowed) != nil || ValidateSearchKeyword(&denied) == nil {
			t.Fatalf("bad boundary for %q", unit)
		}
	}
	if ValidateSearchKeyword(nil) != nil {
		t.Fatal("omitted keyword rejected")
	}
}

func TestOversizedSearchNeverReachesDatabase(t *testing.T) {
	keyword := strings.Repeat("中", 1100)
	// Nil DBs intentionally prove rejection precedes all queries.
	if _, _, err := NewProjectRepository(nil).List(context.Background(), ListParams{Keyword: &keyword}); err == nil {
		t.Fatal("project keyword accepted")
	}
	if _, _, err := NewTalentProfileRepository(nil).List(context.Background(), TalentProfileListParams{Keyword: &keyword}); err == nil {
		t.Fatal("talent keyword accepted")
	}
}

func TestSearchExpansionRemainsBoundedAndArgumentsMatch(t *testing.T) {
	chars := make([]rune, MaxSearchKeywordRunes)
	for i := range chars {
		chars[i] = rune(0x4e00 + i)
	}
	for _, keyword := range []string{"a", "计算机%_!", string(chars)} {
		q := buildDegradedSearchSQLWithMatcher(keyword, projectSearchMatcher)
		if strings.Count(q.Predicate, "?") != len(q.PredicateArgs) || strings.Count(q.Score, "?") != len(q.ScoreArgs) {
			t.Fatal("SQL arguments mismatched")
		}
		if len(q.Predicate)+len(q.Score) > 180000 || len(q.PredicateArgs)+len(q.ScoreArgs) > 1536 {
			t.Fatal("SQL expansion budget exceeded")
		}
	}
}
