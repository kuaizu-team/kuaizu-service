package repository

import (
	"context"
	"os"
	"reflect"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// Opt-in integration test. All fixtures are connection-local temporary tables.
func TestTagRankingMySQL(t *testing.T) {
	dsn := os.Getenv("KUAIZU_TAG_TEST_DSN")
	if dsn == "" {
		t.Skip("set KUAIZU_TAG_TEST_DSN to a disposable MySQL 8+ database")
	}
	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	statements := []string{
		"CREATE TEMPORARY TABLE project_role (code VARCHAR(32), status INT)",
		"CREATE TEMPORARY TABLE talent_role_tag (id INT, tag_text VARCHAR(32), emoji VARCHAR(16), status INT)",
		"CREATE TEMPORARY TABLE talent_role_tag_relation (tag_id INT, role_code VARCHAR(32))",
		"CREATE TEMPORARY TABLE project (id INT, creator_id INT, status INT, deleted_at DATETIME NULL)",
		"CREATE TEMPORARY TABLE project_tag (id INT, name VARCHAR(32), status INT)",
		"CREATE TEMPORARY TABLE project_tag_relation (project_id INT, tag_id INT)",
		"CREATE TEMPORARY TABLE talent_profile (id INT, user_id INT, skill_summary JSON, self_evaluation TEXT, project_experience TEXT, mbti VARCHAR(8), status INT, reject_reason TEXT, view_count INT DEFAULT 0, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)",
		"CREATE TEMPORARY TABLE `user` (id INT, nickname VARCHAR(32), phone VARCHAR(32), email VARCHAR(64), avatar_url TEXT, school_id INT, major_id INT, grade INT, auth_status INT, collaboration_score INT DEFAULT 0)",
		"CREATE TEMPORARY TABLE school (id INT, school_name VARCHAR(32), province VARCHAR(32), city VARCHAR(32), district VARCHAR(32))",
		"CREATE TEMPORARY TABLE major (id INT, major_name VARCHAR(32), class_id INT)",
		"CREATE TEMPORARY TABLE talent_like (talent_profile_id INT)",
		"CREATE TEMPORARY TABLE talent_favorite (talent_profile_id INT)",
		"INSERT INTO project_role VALUES ('tech',1),('ops',1)",
		"INSERT INTO talent_role_tag VALUES (1,'Go','💻',1),(2,'后端','🔧',1)",
		"INSERT INTO talent_role_tag_relation VALUES (1,'tech'),(2,'tech')",
		"INSERT INTO school VALUES (10,'本校','省','市','区'),(20,'外校','外省','外市','外区')",
		"INSERT INTO major VALUES (1,'本专业',1),(2,'其他专业',2)",
		"INSERT INTO `user` (id,nickname,school_id,major_id,auth_status) VALUES (1,'甲',10,1,1),(2,'乙',10,1,1),(3,'丙',10,1,1),(4,'丁',20,1,1),(5,'戊',10,2,1),(6,'己',10,1,0)",
		"INSERT INTO talent_profile (id,user_id,skill_summary,status) VALUES (1,1,'[\"无关\"]',1),(2,2,'[\"💻 Go\",\"自定义\"]',1),(3,3,'[\"后端\"]',1),(4,4,'[\"Go\",\"自定义\"]',1),(5,5,'[\"Go\",\"自定义\"]',1),(6,6,'[\"Go\",\"自定义\"]',1),(70,7,'[\"无关\"]',0)",
		"INSERT INTO project VALUES (100,7,5,NULL),(101,7,1,NULL),(102,7,4,NULL),(103,7,2,NULL)",
		"INSERT INTO project_tag VALUES (1,'Go',1),(2,'自定义',1),(3,'无关',1)",
		"INSERT INTO project_tag_relation VALUES (100,1),(101,2),(102,3),(103,3)",
	}
	for _, s := range statements {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	sortBy := "school_priority"
	school, major := 10, 1
	params := TalentProfileListParams{Page: 1, Size: 20, SortBy: &sortBy, UserSchoolID: &school, UserMajorID: &major, ViewerUserID: 7, RandomSeed: "mysql"}
	repo := NewTalentProfileRepository(db)
	list, total, err := repo.List(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	ids := []int{}
	for _, p := range list {
		ids = append(ids, p.ID)
	}
	if total != 6 || !reflect.DeepEqual(ids, []int{2, 3, 1, 6, 5, 4}) {
		t.Fatalf("tier/tag precedence: total=%d ids=%v", total, ids)
	}
	// Sorting is global before pagination, not per page.
	params.Size = 1
	for page, id := range ids {
		params.Page = page + 1
		rows, _, err := repo.List(ctx, params)
		if err != nil || len(rows) != 1 || rows[0].ID != id {
			t.Fatalf("page %d: rows=%v err=%v", page+1, rows, err)
		}
	}
	// Published tagless projects suppress a nonempty profile fallback.
	if _, err := db.Exec("DELETE FROM project_tag_relation"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE talent_profile SET skill_summary='[]' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	params.Page = 1
	params.Size = 20
	list, _, err = repo.List(ctx, params)
	if err != nil || list[0].ID != 1 || list[1].ID != 3 || list[2].ID != 2 {
		t.Fatalf("missing tags preference: %v %v", list, err)
	}
	// With no published projects the viewer profile becomes the basis.
	if _, err := db.Exec("DELETE FROM project WHERE status IN (1,3,5)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE talent_profile SET skill_summary='[\"Go\"]' WHERE user_id=7"); err != nil {
		t.Fatal(err)
	}
	list, _, err = repo.List(ctx, params)
	if err != nil || list[0].ID != 2 || list[1].ID != 3 {
		t.Fatalf("profile fallback: %v %v", list, err)
	}
	// Project matching reads both decorated library tags and plain custom tags.
	if _, err := db.Exec("INSERT INTO project_tag_relation VALUES (201,1),(202,2)"); err != nil {
		t.Fatal(err)
	}
	candidates := []projectRankCandidate{{ID: 201}, {ID: 202}}
	viewer := 7
	if err := NewProjectRepository(db).scoreProjectTags(ctx, candidates, &viewer); err != nil {
		t.Fatal(err)
	}
	if candidates[0].TagScore.Exact != 1 || candidates[1].TagScore.Exact != 0 {
		t.Fatalf("project scores: %+v", candidates)
	}
}
