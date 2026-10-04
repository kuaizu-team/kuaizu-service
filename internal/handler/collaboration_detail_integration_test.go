package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
)

// Use a fresh disposable database; never an application database. Fixtures are
// real tables because MySQL cannot reopen one temporary table in the same CTE.
func TestCollaborationDetailMySQL(t *testing.T) {
	dsn := os.Getenv("KUAIZU_COLLABORATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set KUAIZU_COLLABORATION_TEST_DSN to a fresh kuaizu_collaboration_test database")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil || cfg.DBName != "kuaizu_collaboration_test" {
		t.Fatal("requires disposable kuaizu_collaboration_test database")
	}
	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		"CREATE TABLE `user` (id INT PRIMARY KEY, collaboration_score DECIMAL(5,2))",
		"CREATE TABLE project (id INT PRIMARY KEY,name VARCHAR(64),creator_id INT,status INT,deleted_at DATETIME NULL,created_at DATETIME DEFAULT CURRENT_TIMESTAMP)",
		"CREATE TABLE project_member_score (project_id INT,member_id INT,score DECIMAL(5,2) NULL)",
		"CREATE TABLE collaboration_score (project_id INT,user_id INT,score DECIMAL(5,2))",
		"CREATE TABLE project_members (project_id INT,user_id INT)",
		"CREATE TABLE project_member_removal (project_id INT,user_id INT)",
		"INSERT INTO `user` VALUES (42,94.25),(7,90)",
		"INSERT INTO project (id,name,creator_id,status) VALUES (1,'当前综合分',42,1),(2,'历史综合分',1,5),(3,'在队未评分',1,1),(4,'离队未评分',1,5),(5,'创建未评分',42,1),(6,'草稿不可见',42,0),(7,'零分',1,1),(8,'重复成员关系',1,1)",
		"INSERT INTO project_member_score VALUES (1,42,80),(1,42,100),(7,42,0),(8,42,NULL)",
		"INSERT INTO collaboration_score VALUES (1,42,20),(2,42,88),(2,42,92)",
		"INSERT INTO project_members VALUES (1,42),(3,42),(7,42),(8,42)",
		"INSERT INTO project_member_removal VALUES (2,42),(4,42),(8,42)",
	}
	for _, q := range statements {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s := NewServer(repository.New(db), nil)
	e := echo.New()
	rec := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	ctx.Set("userID", 7)
	if err := s.GetUserCollaborationDetail(ctx, 42); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var response struct {
		Data struct {
			Projects []collaborationProjectDetail `json:"projects"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	got := map[int]*float64{}
	for _, p := range response.Data.Projects {
		if _, ok := got[p.ProjectID]; ok {
			t.Fatal("duplicate project")
		}
		got[p.ProjectID] = p.Score
	}
	if len(got) != 7 {
		t.Fatalf("projects=%v", got)
	}
	if got[1] == nil || *got[1] != 90 || got[2] == nil || *got[2] != 90 || got[7] == nil || *got[7] != 0 {
		t.Fatalf("aggregate precedence/zero: %+v", response)
	}
	for _, id := range []int{3, 4, 5, 8} {
		if score, ok := got[id]; !ok || score != nil {
			t.Fatalf("project %d must have null score", id)
		}
	}
	if _, ok := got[6]; ok {
		t.Fatal("unpublished project exposed")
	}
}
