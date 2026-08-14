package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestProjectSchoolPriorityUsesHeatUpgradeAndOwnerRoundRobin(t *testing.T) {
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
		"SELECT COUNT(*) FROM project_like pl WHERE pl.project_id = p.id",
		"SELECT COUNT(*) FROM project_favorite pf WHERE pf.project_id = p.id",
		"GREATEST(1,",
		"ROW_NUMBER() OVER (PARTITION BY p.creator_id",
		"CRC32(CONCAT(?, ':owner:', p.creator_id)) ASC",
		"CRC32(CONCAT(?, ':item:', p.id)) ASC, p.id ASC",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q: %s", want, query)
		}
	}
	if len(args) != 6 || args[0].Value != int64(schoolID) || args[1].Value != seed || args[2].Value != seed || args[3].Value != seed || args[4].Value != int64(10) || args[5].Value != int64(0) {
		t.Fatalf("unexpected list args: %#v", args)
	}
}

func TestTalentSchoolPriorityInterleavesAuthByMatchTierAndUsesHeatUpgrade(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewTalentProfileRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQueryQueue(
		captureQueryResult{columns: []string{"count"}, rows: [][]driver.Value{{int64(0)}}},
		captureQueryResult{columns: []string{"id"}},
	)
	sortBy := "school_priority"
	schoolID := 42
	majorID := 77
	majorClassID := 8
	district := "Nanshan"
	city := "Shenzhen"
	province := "Guangdong"
	seed := "7:2026-06-27"

	_, _, err := repo.List(context.Background(), TalentProfileListParams{
		Page: 1, Size: 10, SortBy: &sortBy,
		UserSchoolID:       &schoolID,
		UserMajorID:        &majorID,
		UserMajorClassID:   &majorClassID,
		UserSchoolDistrict: &district,
		UserSchoolCity:     &city,
		UserSchoolProvince: &province,
		RandomSeed:         seed,
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
		"WHEN (u.school_id = ? AND u.major_id = ?) AND u.auth_status = 1 THEN 1",
		"WHEN (u.school_id = ? AND u.major_id = ?) THEN 2",
		"WHEN (u.school_id = ? AND tm.class_id = ?) AND u.auth_status = 1 THEN 3",
		"WHEN (u.school_id = ?) THEN 6",
		"WHEN (ts.district = ? AND ts.city = ? AND u.major_id = ?) AND u.auth_status = 1 THEN 7",
		"WHEN (ts.city = ? AND u.major_id = ?) AND u.auth_status = 1 THEN 13",
		"WHEN (ts.province = ? AND u.major_id = ?) AND u.auth_status = 1 THEN 19",
		"WHEN (1 = 1 AND u.major_id = ?) AND u.auth_status = 1 THEN 25",
		"WHEN (1 = 1) THEN 30",
		"SELECT COUNT(*) FROM talent_like tl WHERE tl.talent_profile_id = tp.id",
		"SELECT COUNT(*) FROM talent_favorite tf WHERE tf.talent_profile_id = tp.id",
		"GREATEST(1, (CASE",
		"CRC32(CONCAT(?, ':item:', tp.id)) ASC, tp.id ASC",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q: %s", want, query)
		}
	}
	if strings.Count(query, "WHEN (u.school_id = ? AND u.major_id = ?) AND u.auth_status = 1 THEN 1") != 2 {
		t.Fatalf("base tier must be reused as the native-before-promoted tie-breaker: %s", query)
	}
	if strings.Contains(query, "ROW_NUMBER() OVER (PARTITION BY tp.user_id") {
		t.Fatalf("unique talent user must not use a redundant window rank: %s", query)
	}
	orderPos := strings.Index(query, "ORDER BY ")
	if orderPos < 0 {
		t.Fatalf("query missing ORDER BY: %s", query)
	}
	if strings.HasPrefix(query[orderPos+len("ORDER BY "):], "CASE WHEN u.auth_status") {
		t.Fatalf("certification must be part of each match tier, not a global prefix: %s", query)
	}
	if len(args) != 103 || args[len(args)-3].Value != seed || args[len(args)-2].Value != int64(10) || args[len(args)-1].Value != int64(0) {
		t.Fatalf("unexpected list args: %#v", args)
	}
	if strings.Count(queries[1], "?") != len(args) {
		t.Fatalf("placeholder count does not match args: placeholders=%d args=%d", strings.Count(queries[1], "?"), len(args))
	}
}

func capturedQueriesAndArgs() ([]string, []driver.NamedValue) {
	capturedQuery.Lock()
	defer capturedQuery.Unlock()
	return append([]string(nil), capturedQuery.queries...), append([]driver.NamedValue(nil), capturedQuery.args...)
}
