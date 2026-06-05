package repository

import (
	"context"
	"database/sql/driver"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

var capturedQuery struct {
	sync.Mutex
	query   string
	queries []string
	args    []driver.NamedValue
	columns []string
	rows    [][]driver.Value
	queue   []captureQueryResult
}

type captureQueryResult struct {
	columns []string
	rows    [][]driver.Value
}

func (captureConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	capturedQuery.Lock()
	defer capturedQuery.Unlock()
	capturedQuery.query = query
	capturedQuery.queries = append(capturedQuery.queries, query)
	capturedQuery.args = append([]driver.NamedValue(nil), args...)
	result := captureQueryResult{columns: capturedQuery.columns, rows: capturedQuery.rows}
	if len(capturedQuery.queue) > 0 {
		result = capturedQuery.queue[0]
		capturedQuery.queue = capturedQuery.queue[1:]
	}
	rows := make([][]driver.Value, len(result.rows))
	for i := range result.rows {
		rows[i] = append([]driver.Value(nil), result.rows[i]...)
	}
	return &captureRows{columns: append([]string(nil), result.columns...), rows: rows}, nil
}

type captureRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (r *captureRows) Columns() []string { return r.columns }
func (r *captureRows) Close() error      { return nil }
func (r *captureRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.index])
	r.index++
	return nil
}

func TestInteractionTables(t *testing.T) {
	tests := []struct {
		target, like, id string
	}{
		{InteractionProject, "project_like", "project_id"},
		{InteractionTalent, "talent_like", "talent_profile_id"},
	}
	for _, tt := range tests {
		like, _, _, id, err := interactionTables(tt.target)
		if err != nil || like != tt.like || id != tt.id {
			t.Fatalf("interactionTables(%q) = %q, %q, %v", tt.target, like, id, err)
		}
	}
	if _, _, _, _, err := interactionTables("bad"); err == nil {
		t.Fatal("expected invalid target error")
	}
}

func TestUnreadForTargetCalculatesThreeTypesAndDefaultState(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewInteractionRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQuery([]string{"like_count", "favorite_count", "share_count"}, [][]driver.Value{{int64(2), int64(1), int64(3)}})

	got, err := repo.UnreadForTarget(context.Background(), InteractionProject, 42, 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.LikeCount != 2 || got.FavoriteCount != 1 || got.ShareCount != 3 || got.TotalCount != 6 {
		t.Fatalf("unexpected unread: %#v", got)
	}
	capturedQuery.Lock()
	query := normalizeSQL(capturedQuery.query)
	capturedQuery.Unlock()
	for _, want := range []string{"project_like", "project_favorite", "project_share", "interaction_dashboard_view_state", "created_at>COALESCE", "1970-01-01 00:00:01"} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q: %s", want, query)
		}
	}
}

func TestUnreadDashboardTotals(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewInteractionRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQuery([]string{"project_count", "talent_count"}, [][]driver.Value{{int64(6), int64(2)}})

	got, err := repo.UnreadDashboardTotals(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProjectCount != 6 || got.TalentCount != 2 || got.TotalCount != 8 {
		t.Fatalf("unexpected totals: %#v", got)
	}
}

func TestBatchProjectUnreadUsesOneAggregateQuery(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewInteractionRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQuery([]string{"project_id", "unread_count"}, [][]driver.Value{{int64(10), int64(6)}, {int64(11), int64(2)}})

	got, err := repo.BatchProjectUnread(context.Background(), 7, []int{10, 11})
	if err != nil {
		t.Fatal(err)
	}
	if got[10] != 6 || got[11] != 2 {
		t.Fatalf("unexpected batch unread: %#v", got)
	}
	capturedQuery.Lock()
	query := normalizeSQL(capturedQuery.query)
	capturedQuery.Unlock()
	if strings.Count(query, "UNION ALL") != 2 || !strings.Contains(query, "GROUP BY x.project_id") {
		t.Fatalf("expected one three-type aggregate query: %s", query)
	}
}

func setCapturedQuery(columns []string, rows [][]driver.Value) {
	capturedQuery.Lock()
	capturedQuery.query = ""
	capturedQuery.queries = nil
	capturedQuery.args = nil
	capturedQuery.columns = columns
	capturedQuery.rows = rows
	capturedQuery.queue = nil
	capturedQuery.Unlock()
}

func setCapturedQueryQueue(results ...captureQueryResult) {
	capturedQuery.Lock()
	capturedQuery.query = ""
	capturedQuery.queries = nil
	capturedQuery.args = nil
	capturedQuery.columns = nil
	capturedQuery.rows = nil
	capturedQuery.queue = append([]captureQueryResult(nil), results...)
	capturedQuery.Unlock()
}

func TestListFavoriteTalentsBatchEnrichesAndExcludesPrivateFields(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewInteractionRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQueryQueue(
		captureQueryResult{columns: []string{"count"}, rows: [][]driver.Value{{int64(1)}}},
		captureQueryResult{
			columns: []string{"id", "user_id", "self_evaluation", "skill_summary", "project_experience", "mbti", "status", "view_count", "created_at", "updated_at", "nickname", "avatar_url", "school_id", "major_id", "grade", "auth_status", "favorited_at"},
			rows:    [][]driver.Value{{int64(9), int64(5), nil, `["Go"]`, nil, "INTJ", int64(1), int64(3), nil, nil, "Alice", "avatar", int64(2), int64(4), int64(2024), int64(1), time.Date(2026, 6, 5, 10, 0, 0, 0, time.Local)}},
		},
		captureQueryResult{columns: []string{"id", "school_name"}, rows: [][]driver.Value{{int64(2), "北京大学"}}},
		captureQueryResult{columns: []string{"id", "major_name"}, rows: [][]driver.Value{{int64(4), "计算机科学"}}},
	)

	items, total, err := repo.ListFavoriteTalents(context.Background(), 7, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].SchoolName == nil || *items[0].SchoolName != "北京大学" || items[0].MajorName == nil || *items[0].MajorName != "计算机科学" {
		t.Fatalf("unexpected favorites: total=%d items=%#v", total, items)
	}
	if items[0].Phone != nil || items[0].Email != nil || items[0].WechatID != nil {
		t.Fatalf("private fields must not be selected: %#v", items[0].TalentProfile)
	}
	capturedQuery.Lock()
	queries := append([]string(nil), capturedQuery.queries...)
	capturedQuery.Unlock()
	if len(queries) != 4 || strings.Contains(queries[1], "u.phone") || strings.Contains(queries[1], "u.email") || !strings.Contains(queries[1], "ORDER BY f.created_at DESC") {
		t.Fatalf("unexpected favorite talent queries: %#v", queries)
	}
}

func TestListFavoriteTalentsSkipsOrphansAndReturnsEmptyPage(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewInteractionRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQueryQueue(
		captureQueryResult{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}},
		captureQueryResult{columns: []string{"id", "user_id", "self_evaluation", "skill_summary", "project_experience", "mbti", "status", "view_count", "created_at", "updated_at", "nickname", "avatar_url", "school_id", "major_id", "grade", "auth_status", "favorited_at"}},
	)

	items, total, err := repo.ListFavoriteTalents(context.Background(), 7, 2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("unexpected empty favorites: total=%d items=%#v", total, items)
	}
	capturedQuery.Lock()
	queries := append([]string(nil), capturedQuery.queries...)
	args := append([]driver.NamedValue(nil), capturedQuery.args...)
	capturedQuery.Unlock()
	if !strings.Contains(queries[0], "JOIN talent_profile") || len(args) != 3 || args[2].Value != int64(10) {
		t.Fatalf("orphan/pagination query mismatch: queries=%#v args=%#v", queries, args)
	}
}

func TestMarkDashboardViewedAllTypes(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewInteractionRepository(sqlx.NewDb(db, "capture_user_repo"))

	capturedExec.Lock()
	capturedExec.query, capturedExec.args = "", nil
	capturedExec.Unlock()

	if err := repo.MarkDashboardViewed(context.Background(), 7, InteractionProject, 42, nil); err != nil {
		t.Fatalf("MarkDashboardViewed returned error: %v", err)
	}
	capturedExec.Lock()
	query := normalizeSQL(capturedExec.query)
	args := append([]driver.NamedValue(nil), capturedExec.args...)
	capturedExec.Unlock()

	if len(args) != 12 {
		t.Fatalf("args count = %d, want 12", len(args))
	}
	if args[3].Value != "like" || args[7].Value != "favorite" || args[11].Value != "share" {
		t.Fatalf("interaction type args = %#v", args)
	}
	if want := "ON DUPLICATE KEY UPDATE last_viewed_at=VALUES(last_viewed_at),updated_at=NOW()"; !containsNormalized(query, want) {
		t.Fatalf("query = %q, want %q", query, want)
	}
}

func TestMarkDashboardViewedSingleType(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewInteractionRepository(sqlx.NewDb(db, "capture_user_repo"))
	kind := "share"

	capturedExec.Lock()
	capturedExec.query, capturedExec.args = "", nil
	capturedExec.Unlock()

	if err := repo.MarkDashboardViewed(context.Background(), 7, InteractionTalent, 9, &kind); err != nil {
		t.Fatalf("MarkDashboardViewed returned error: %v", err)
	}
	capturedExec.Lock()
	args := append([]driver.NamedValue(nil), capturedExec.args...)
	capturedExec.Unlock()
	if len(args) != 4 || args[1].Value != InteractionTalent || args[2].Value != int64(9) || args[3].Value != "share" {
		t.Fatalf("args = %#v", args)
	}
}

func containsNormalized(query, want string) bool {
	return strings.Contains(normalizeSQL(query), normalizeSQL(want))
}
