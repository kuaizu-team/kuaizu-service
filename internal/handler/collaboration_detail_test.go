package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
)

func TestCollaborationDetailRequiresLogin(t *testing.T) {
	e := echo.New()
	api.RegisterHandlers(e, &Server{})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/42/collaboration-detail", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCollaborationDetailPublicAggregate(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	server := NewServer(repository.New(sqlx.NewDb(raw, "mysql")), nil)
	mock.ExpectQuery("SELECT COALESCE").WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"score"}).AddRow(94.25))
	mock.ExpectQuery("(?s)WITH scored AS.*ROUND\\(AVG\\(pms.score\\),2\\).*NOT EXISTS.*project_members.*project_member_removal.*creator_id").WithArgs(42, 42, 42, 42, 42).WillReturnRows(sqlmock.NewRows([]string{"project_id", "project_name", "score"}).AddRow(10, "已评分项目", 88.75).AddRow(11, "未评分项目", nil).AddRow(12, "零分项目", 0))
	e := echo.New()
	rec := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/users/42/collaboration-detail", nil), rec)
	ctx.Set("userID", 7) // An authenticated viewer may read another user's aggregates.
	if err := server.GetUserCollaborationDetail(ctx, 42); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var response struct {
		Data struct {
			Score    float64                      `json:"score"`
			Level    string                       `json:"level"`
			Ranges   []collaborationRange         `json:"ranges"`
			Projects []collaborationProjectDetail `json:"projects"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Score != 94.25 || response.Data.Level != "优秀" || len(response.Data.Projects) != 3 {
		t.Fatal(rec.Body.String())
	}
	if response.Data.Projects[1].Score != nil || *response.Data.Projects[2].Score != 0 {
		t.Fatal("null must differ from zero")
	}
	for _, field := range []string{"rater", "ratingId", "phone", "email", "authImg"} {
		if strings.Contains(rec.Body.String(), field) {
			t.Fatalf("unexpected private field %s", field)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCollaborationDetailMissingUser(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	mock.ExpectQuery("SELECT COALESCE").WithArgs(42).WillReturnError(sql.ErrNoRows)
	server := NewServer(repository.New(sqlx.NewDb(raw, "mysql")), nil)
	e := echo.New()
	rec := httptest.NewRecorder()
	ctx := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	ctx.Set("userID", 7)
	if err := server.GetUserCollaborationDetail(ctx, 42); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 404 {
		t.Fatal(rec.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCollaborationRangesMatchModelAtBoundaries(t *testing.T) {
	ranges := collaborationRanges()
	for _, score := range []float64{0, 49.99, 50, 84.99, 85, 89.99, 90, 94.99, 95, 100} {
		count := 0
		for _, r := range ranges {
			if score >= r.Min && (score < r.Max || r.MaxInclusive && score == r.Max) {
				count++
				if r.Level != models.CollaborationLevel(score) {
					t.Fatalf("mismatched level at %v", score)
				}
			}
		}
		if count != 1 {
			t.Fatalf("score %v belongs to %d ranges", score, count)
		}
	}
}
