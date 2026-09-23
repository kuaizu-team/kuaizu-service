package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"reflect"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func testTagMatcher() *tagMatcher {
	return newTagMatcher([]matchingTag{
		{Text: "Go", Emoji: "💻", Role: "tech"},
		{Text: "后端", Emoji: "🔧", Role: "tech"},
		{Text: "运营", Emoji: "📣", Role: "ops"},
		{Text: "内容", Emoji: "📝", Role: "ops"},
		{Text: "沟通", Emoji: "🤝", Role: "tech"},
		{Text: "沟通", Emoji: "🤝", Role: "ops"},
	}, 2)
}

func TestTagMatchingNormalizationAndRoles(t *testing.T) {
	m := testTagMatcher()
	basis := m.basis([]string{"💻 Go", "go", "  自定义  ", "📣 运营"})
	got := m.score(basis, []string{"GO", "💻Go", "自定义", "陌生", ""})
	if got.Exact != 2 || got.Count != 3 {
		t.Fatalf("dedup/custom match: %+v", got)
	}
	if m.score(basis, []string{"完全不同"}).Role != 0 {
		t.Fatal("custom tags must not infer roles")
	}
	if m.score(m.basis([]string{"沟通"}), []string{"后端"}).Role != 0 {
		t.Fatal("generic tags must not infer roles")
	}
	mixed := m.score(basis, []string{"后端", "内容"})
	single := m.score(basis, []string{"后端"})
	if mixed.Role != 1000000 || mixed.Role <= single.Role {
		t.Fatalf("multi-role preference: mixed=%+v single=%+v", mixed, single)
	}
	if m.score(m.basis(nil), []string{"", "Go", "💻 Go"}).Count != 1 {
		t.Fatal("missing-tag preference uses distinct nonempty tags")
	}
}

func TestTalentReferencePriority(t *testing.T) {
	for _, tc := range []struct {
		name       string
		project    *string
		hasProject bool
		profile    string
		want       []string
	}{
		{"project", tagString("项目标签"), true, "", []string{"项目标签"}},
		{"tagless project", nil, true, "", nil},
		{"profile fallback", nil, false, `["名片标签"]`, []string{"名片标签"}},
		{"missing profile", nil, false, "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()
			rows := sqlmock.NewRows([]string{"name"})
			if tc.hasProject {
				if tc.project == nil {
					rows.AddRow(nil)
				} else {
					rows.AddRow(*tc.project)
				}
			}
			mock.ExpectQuery(`SELECT t.name FROM project p`).WithArgs(7).WillReturnRows(rows)
			if !tc.hasProject {
				expected := mock.ExpectQuery(`SELECT skill_summary FROM talent_profile WHERE user_id = \?`).WithArgs(7)
				if tc.profile == "" {
					expected.WillReturnError(sql.ErrNoRows)
				} else {
					expected.WillReturnRows(sqlmock.NewRows([]string{"skill_summary"}).AddRow(tc.profile))
				}
			}
			got, err := talentReferenceTags(context.Background(), sqlx.NewDb(raw, "mysql"), 7)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, %v; want %v", got, err, tc.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func tagString(s string) *string { return &s }

func TestProjectTagsPreservePoolMembershipAndMajor(t *testing.T) {
	user, school, major := 7, 1, 10
	params := ListParams{ViewerUserID: &user, UserSchoolID: &school, UserMajorID: &major, RandomSeed: "test"}
	input := []projectRankCandidate{}
	for i := 1; i <= 12; i++ {
		input = append(input, projectRankCandidate{ID: i, CreatorID: i, SchoolID: 1 + (i-1)/6, MajorID: &major})
	}
	original := rankProjectCandidates(append([]projectRankCandidate(nil), input...), params)
	for i := range input {
		input[i].TagScore = tagMatchScore{Exact: i % 6, Role: 1000000 - i}
	}
	ranked := rankProjectCandidates(input, params)
	tiers := map[int]int{}
	for _, c := range original {
		tiers[c.ID] = c.Tier
	}
	for i, c := range ranked {
		if c.Tier != tiers[c.ID] {
			t.Fatal("tag score changed pool membership")
		}
		if i > 0 && ranked[i-1].Tier == c.Tier && ranked[i-1].TagScore.Exact < c.TagScore.Exact {
			t.Fatal("exact overlap must sort descending within pool")
		}
	}
	// Borrowing and singleton exchange must select the same IDs with or without tags.
	for size := 2; size < 9; size++ {
		candidates := append([]projectRankCandidate(nil), input[:size]...)
		for i := range candidates {
			candidates[i].SchoolID = i + 1
			candidates[i].TagScore = tagMatchScore{}
		}
		before := rankProjectCandidates(append([]projectRankCandidate(nil), candidates...), params)
		for i := range candidates {
			candidates[i].TagScore.Exact = i + 1
		}
		after := rankProjectCandidates(candidates, params)
		for _, a := range after {
			for _, b := range before {
				if a.ID == b.ID && a.Tier != b.Tier {
					t.Fatal("tags changed borrowing/exchange")
				}
			}
		}
	}
}

func TestTalentScoresJoinBeforePagination(t *testing.T) {
	for _, missing := range []bool{false, true} {
		raw, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		db := sqlx.NewDb(raw, "mysql")
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM talent_profile`).WithArgs(3).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
		ref := sqlmock.NewRows([]string{"name"})
		if missing {
			ref.AddRow(nil)
		} else {
			ref.AddRow("Go")
		}
		mock.ExpectQuery(`SELECT t.name FROM project p`).WithArgs(7).WillReturnRows(ref)
		mock.ExpectQuery(`SELECT t.tag_text`).WillReturnRows(sqlmock.NewRows([]string{"tag_text", "emoji", "role_code"}).AddRow("Go", "💻", "tech"))
		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM project_role`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
		mock.ExpectQuery("SELECT tp.id, tp.skill_summary FROM talent_profile").WithArgs(3).WillReturnRows(sqlmock.NewRows([]string{"id", "skill_summary"}).AddRow(10, `["💻 Go"]`).AddRow(11, `[]`))
		order := "COALESCE(tag_scores.exact_score, 0) DESC, COALESCE(tag_scores.role_score, 0) DESC, "
		if missing {
			order = "COALESCE(tag_scores.tag_count, 0) ASC, "
		}
		pattern := `(?s)SELECT.*LEFT JOIN JSON_TABLE\(\?,.*WHERE tp.status = 1 AND u.school_id = \?.*ORDER BY GREATEST.*` + regexp.QuoteMeta(order) + `CRC32.*LIMIT \? OFFSET \?`
		mock.ExpectQuery(pattern).WithArgs(tagScoreJSON{missing: missing}, 3, "seed", 1, 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
		sortBy := "school_priority"
		school := 3
		_, _, err = NewTalentProfileRepository(db).List(context.Background(), TalentProfileListParams{Page: 2, Size: 1, SchoolID: &school, SortBy: &sortBy, ViewerUserID: 7, RandomSeed: "seed"})
		if err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		raw.Close()
	}
}

type tagScoreJSON struct{ missing bool }

func (m tagScoreJSON) Match(value driver.Value) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	var rows []struct {
		ID int `json:"id"`
		tagMatchScore
	}
	if json.Unmarshal([]byte(text), &rows) != nil || len(rows) != 2 {
		return false
	}
	exact := 1
	if m.missing {
		exact = 0
	}
	return rows[0].ID == 10 && rows[0].Exact == exact && rows[0].Count == 1 && rows[1].ID == 11 && rows[1].Count == 0
}

func TestProjectTagSortPreservesSlotsAndTies(t *testing.T) {
	items := []projectRankCandidate{
		{ID: 1, Tier: 1, MajorMatch: 0},
		{ID: 2, Tier: 1, MajorMatch: 1, TagScore: tagMatchScore{Exact: 9}},
		{ID: 3, Tier: 2, MajorMatch: 0, TagScore: tagMatchScore{Exact: 9}},
		{ID: 4, Tier: 1, MajorMatch: 0, TagScore: tagMatchScore{Exact: 1}},
		{ID: 5, Tier: 1, MajorMatch: 0, TagScore: tagMatchScore{Role: 1000000}},
	}
	sortProjectTagsWithinPools(items)
	want := []int{4, 2, 3, 5, 1}
	for i, c := range items {
		if c.ID != want[i] {
			t.Fatalf("slots/exact precedence: %+v", items)
		}
	}
	for i := range items {
		items[i].TagScore = tagMatchScore{}
	}
	before := append([]projectRankCandidate(nil), items...)
	sortProjectTagsWithinPools(items)
	if !reflect.DeepEqual(items, before) {
		t.Fatal("no tags must retain legacy order exactly")
	}
}

func TestProjectWithoutViewerTagsSkipsLibraryAndCandidateQueries(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	mock.ExpectQuery(`SELECT skill_summary FROM talent_profile`).WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"skill_summary"}).AddRow(`[]`))
	viewer := 7
	candidates := []projectRankCandidate{{ID: 1}}
	if err := NewProjectRepository(sqlx.NewDb(raw, "mysql")).scoreProjectTags(context.Background(), candidates, &viewer); err != nil {
		t.Fatal(err)
	}
	if candidates[0].TagScore != (tagMatchScore{}) {
		t.Fatal("empty profile changed ranking score")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkTagMatching1000Candidates(b *testing.B) {
	matcher := testTagMatcher()
	basis := matcher.basis([]string{"Go", "运营", "自定义"})
	tags := []string{"💻 Go", "后端", "内容", "沟通", "自定义"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 1000; j++ {
			matcher.score(basis, tags)
		}
	}
}
