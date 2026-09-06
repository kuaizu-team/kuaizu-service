package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestProjectSearchScorePrecedesExistingPoolOrder(t *testing.T) {
	keyword := "46级"
	params := ListParams{Keyword: &keyword, RandomSeed: "search-ranking"}
	candidates := []projectRankCandidate{
		{ID: 1, CreatorID: 1, SchoolID: 1, SearchScore: 1},
		{ID: 2, CreatorID: 2, SchoolID: 1, SearchScore: 4},
		{ID: 3, CreatorID: 3, SchoolID: 1, SearchScore: 2},
		{ID: 4, CreatorID: 4, SchoolID: 1, SearchScore: 4},
	}

	ranked := rankProjectCandidates(candidates, params)
	for i := 1; i < len(ranked); i++ {
		if ranked[i-1].SearchScore < ranked[i].SearchScore {
			t.Fatalf("search scores not descending: %#v", ranked)
		}
	}
}

func TestProjectEventScorePrecedesKeywordAndExistingPoolOrder(t *testing.T) {
	eventIDs := []int{1, 2}
	keyword := "创新"
	params := ListParams{EventIDs: eventIDs, Keyword: &keyword, RandomSeed: "event-ranking"}
	candidates := []projectRankCandidate{
		{ID: 1, CreatorID: 1, SearchScore: 4, EventCategory: 2, EventMatches: 2},
		{ID: 2, CreatorID: 2, SearchScore: 1, EventCategory: 4, EventRelations: 1},
		{ID: 3, CreatorID: 3, SearchScore: 4, EventCategory: 3, EventMatches: 1},
		{ID: 4, CreatorID: 4, SearchScore: 4, EventCategory: 3, EventMatches: 2},
	}

	ranked := rankProjectCandidates(candidates, params)
	want := []int{2, 4, 3, 1}
	for i := range want {
		if ranked[i].ID != want[i] {
			t.Fatalf("ranked[%d] = %d, want %d: %#v", i, ranked[i].ID, want[i], ranked)
		}
	}
}

func TestProjectCompositeFavorites(t *testing.T) {
	candidate := projectRankCandidate{FavoriteCount: 9, LikeCount: 14, ShareCount: 29}
	if got := projectCompositeFavorites(candidate); got != 13 {
		t.Fatalf("composite favorites = %d, want 13", got)
	}
}

func TestProjectGeoAndMajorMatch(t *testing.T) {
	viewerID, schoolID, majorID, classID := 7, 10, 100, 20
	province, city, district := "广东省", "深圳市", "南山区"
	params := ListParams{
		ViewerUserID:       &viewerID,
		UserSchoolID:       &schoolID,
		UserSchoolProvince: &province,
		UserSchoolCity:     &city,
		UserSchoolDistrict: &district,
		UserMajorID:        &majorID,
		UserMajorClassID:   &classID,
	}

	nearMajor, otherSchool := 101, 11
	candidate := projectRankCandidate{
		SchoolID: otherSchool, Province: &province, City: &city, District: &district,
		MajorID: &nearMajor, MajorClassID: &classID,
	}
	if got := projectGeoTier(candidate, params); got != 2 {
		t.Fatalf("geo tier = %d, want 2", got)
	}
	if got := projectMajorMatch(candidate, params); got != 1 {
		t.Fatalf("major match = %d, want near-major rank 1", got)
	}

	params.ViewerUserID = nil
	if got := projectGeoTier(candidate, params); got != 1 {
		t.Fatalf("anonymous geo tier = %d, want fully-random tier 1", got)
	}
	if got := projectMajorMatch(candidate, params); got != 0 {
		t.Fatalf("anonymous major match = %d, want neutral rank 0", got)
	}
}

func TestAvoidAdjacentProjectOwners(t *testing.T) {
	items := []projectRankCandidate{
		{ID: 1, CreatorID: 1}, {ID: 2, CreatorID: 1},
		{ID: 3, CreatorID: 2}, {ID: 4, CreatorID: 3},
		{ID: 5, CreatorID: 1}, {ID: 6, CreatorID: 2},
	}
	got := avoidAdjacentProjectOwners(items)
	for i := 1; i < len(got); i++ {
		if got[i-1].CreatorID == got[i].CreatorID {
			t.Fatalf("adjacent creator %d at indexes %d and %d: %#v", got[i].CreatorID, i-1, i, got)
		}
	}
}

func TestAvoidAdjacentProjectOwnersKeepsBestPossibleWhenImpossible(t *testing.T) {
	items := []projectRankCandidate{
		{ID: 1, CreatorID: 1}, {ID: 2, CreatorID: 1},
		{ID: 3, CreatorID: 1}, {ID: 4, CreatorID: 2},
	}
	got := avoidAdjacentProjectOwners(items)
	adjacent := 0
	for i := 1; i < len(got); i++ {
		if got[i-1].CreatorID == got[i].CreatorID {
			adjacent++
		}
	}
	if adjacent != 1 {
		t.Fatalf("unavoidable adjacency count = %d, want 1: %#v", adjacent, got)
	}
}

func TestRankProjectCandidatesIsStableForSeed(t *testing.T) {
	candidates := make([]projectRankCandidate, 12)
	for i := range candidates {
		candidates[i] = projectRankCandidate{ID: i + 1, CreatorID: i + 1, SchoolID: i + 1}
	}
	params := ListParams{RandomSeed: "stable-seed"}
	first := rankProjectCandidates(append([]projectRankCandidate(nil), candidates...), params)
	second := rankProjectCandidates(append([]projectRankCandidate(nil), candidates...), params)
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("same seed changed index %d: %d != %d", i, first[i].ID, second[i].ID)
		}
	}
}

func TestSingletonPoolChangesPositionEveryRefresh(t *testing.T) {
	base := [][]projectRankCandidate{
		{{ID: 1, CreatorID: 1, Tier: 1}},
		{{ID: 2, CreatorID: 2, Tier: 2}},
		{}, {}, {},
	}
	positions := make([]int, 3)
	for sequence := 0; sequence < 3; sequence++ {
		pools := make([][]projectRankCandidate, len(base))
		for i := range base {
			pools[i] = append([]projectRankCandidate(nil), base[i]...)
		}
		shuffleSingletonProjectPools(pools, "r"+string(rune('0'+sequence))+":seed")
		ordered := flattenProjectPools(pools)
		for index, item := range ordered {
			if item.ID == 1 {
				positions[sequence] = index
			}
		}
	}
	if positions[0] == positions[1] || positions[1] == positions[2] {
		t.Fatalf("singleton positions did not change on each refresh: %v", positions)
	}
}

func TestProjectPromotedTier(t *testing.T) {
	if got := projectPromotedTier(4, 9); got != 4 {
		t.Fatalf("tier with 9 composite favorites = %d, want 4", got)
	}
	if got := projectPromotedTier(4, 10); got != 3 {
		t.Fatalf("tier with 10 composite favorites = %d, want 3", got)
	}
	if got := projectPromotedTier(5, 100); got != 1 {
		t.Fatalf("tier cap = %d, want 1", got)
	}
}

func TestProjectRankedPageLoadsOnlySelectedIDsWithoutSecondOffset(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewProjectRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQueryQueue(
		captureQueryResult{columns: []string{"count"}, rows: [][]driver.Value{{int64(1)}}},
		captureQueryResult{
			columns: []string{"id", "creator_id", "school_id", "province", "city", "district", "major_id", "major_class_id", "like_count", "favorite_count", "share_count"},
			rows:    [][]driver.Value{{int64(91), int64(7), int64(3), nil, nil, nil, nil, nil, int64(0), int64(0), int64(0)}},
		},
		captureQueryResult{columns: []string{"id"}},
	)
	sortBy := "school_priority"
	_, total, err := repo.List(context.Background(), ListParams{Page: 1, Size: 10, SortBy: &sortBy, RandomSeed: "page-seed"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}

	queries, args := capturedQueriesAndArgs()
	if len(queries) != 3 {
		t.Fatalf("query count = %d, want 3", len(queries))
	}
	query := normalizeSQL(queries[2])
	if !strings.Contains(query, "WHERE p.id IN (?) ORDER BY FIELD(p.id, ?)") {
		t.Fatalf("ranked page query does not preserve selected ID order: %s", query)
	}
	if len(args) != 4 || args[0].Value != int64(91) || args[1].Value != int64(91) || args[2].Value != int64(10) || args[3].Value != int64(0) {
		t.Fatalf("unexpected ranked page args: %#v", args)
	}
}
