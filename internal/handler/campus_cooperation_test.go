package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

type cooperationUserStub struct {
	repository.UserRepo
	user *models.User
}

func (f cooperationUserStub) GetByID(_ context.Context, id int) (*models.User, error) {
	return f.user, nil
}
func TestCooperationDetailHTTPDoesNotLeakPendingContact(t *testing.T) {
	raw, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	db := sqlx.NewDb(raw, "sqlmock")
	school := 42
	repo := repository.New(db)
	repo.User = cooperationUserStub{user: &models.User{ID: 99, SchoolID: &school}}
	svc := service.NewCampusCooperationService(repo, nil, nil)
	server := NewServer(repo, &service.Services{CampusCooperation: svc})
	m.ExpectQuery(`(?s)SELECT c.id.*WHERE c.id=\?`).WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{"id", "school_id", "published", "application_required", "contacts_visible", "contacts", "links_visible", "links"}).AddRow(9, 42, true, true, true, `[{"type":"wechat","value":"private-contact"}]`, true, `[{"title":"公开链接","url":"https://example.org"}]`))
	m.ExpectQuery(`SELECT EXISTS`).WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	m.ExpectQuery(`(?s)SELECT id,cooperation_id.*WHERE cooperation_id=\? AND user_id=\?`).WithArgs(int64(9), 99).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("pending"))
	req := httptest.NewRequest(http.MethodGet, "/api/v2/campus-cooperations/9?userId=123", nil)
	rec := httptest.NewRecorder()
	ctx := echo.New().NewContext(req, rec)
	ctx.Set("userID", 99)
	ctx.SetParamNames("id")
	ctx.SetParamValues("9")
	if err := server.GetCampusCooperation(ctx); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "private-contact") || !strings.Contains(rec.Body.String(), "公开链接") {
		t.Fatalf("HTTP projection: %d %s", rec.Code, rec.Body)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestCooperationMessageCountRequiresExplicitCapability(t *testing.T) {
	for _, include := range []bool{false, true} {
		t.Run(fmt.Sprint(include), func(t *testing.T) {
			raw, m, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()
			repo := repository.New(sqlx.NewDb(raw, "sqlmock"))
			server := NewServer(repo, nil)
			query := `SELECT COUNT(*) FROM user_status_message WHERE user_id=? AND read_at IS NULL`
			url := "/api/v2/users/me/messages/unread-count"
			if !include {
				query += " AND type NOT IN ('cooperation-approved','cooperation-rejected')"
			} else {
				url += "?includeCooperation=true"
			}
			m.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(99).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
			rec := httptest.NewRecorder()
			ctx := echo.New().NewContext(httptest.NewRequest(http.MethodGet, url, nil), rec)
			ctx.Set("userID", 99)
			if err := server.GetMyUnreadStatusMessageCount(ctx); err != nil {
				t.Fatal(err)
			}
			var data struct {
				Data struct {
					Count int `json:"count"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil || data.Data.Count != 2 {
				t.Fatalf("%s %v", rec.Body, err)
			}
			if err := m.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestCooperationInvalidMessageCapabilityIsRejected(t *testing.T) {
	ctx := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/?includeCooperation=maybe", nil), httptest.NewRecorder())
	if _, err := cooperationClientCapability(ctx); err == nil {
		t.Fatal("invalid capability silently accepted")
	}
}

func TestCooperationInvalidParametersStopBeforeDatabaseAccess(t *testing.T) {
	for _, url := range []string{"/campus-cooperations?page=bad", "/campus-cooperations?size=0", "/users/me/messages?includeCooperation=bad"} {
		rec := httptest.NewRecorder()
		ctx := echo.New().NewContext(httptest.NewRequest(http.MethodGet, url, nil), rec)
		ctx.Set("userID", 99)
		server := &Server{} // Any accidental business call would panic on these nil dependencies.
		var err error
		if strings.HasPrefix(url, "/users") {
			err = server.ListMyStatusMessages(ctx)
		} else {
			err = server.ListCampusCooperations(ctx)
		}
		if err != nil || rec.Code != 400 {
			t.Fatalf("%s: status=%d err=%v", url, rec.Code, err)
		}
	}
}
