package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/auth"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

type resourceAccessUserRepo struct {
	repository.UserRepo
	user *models.User
	err  error
}

func (r resourceAccessUserRepo) GetByID(context.Context, int) (*models.User, error) {
	return r.user, r.err
}

type resourceAccessEventRepo struct {
	repository.EventRepo
	event *models.Event
}

func (r resourceAccessEventRepo) GetByID(context.Context, int) (*models.Event, error) {
	return r.event, nil
}
func (r resourceAccessEventRepo) ListTimelineNodes(context.Context, int) ([]models.EventTimelineNode, error) {
	return nil, nil
}
func (r resourceAccessEventRepo) ListProjectIDs(context.Context, int) ([]int, error) { return nil, nil }
func (r resourceAccessEventRepo) IncrementViewCount(context.Context, int) error      { return nil }

func TestEventResourcesRequireCurrentStudentCertification(t *testing.T) {
	t.Setenv("JWT_SECRET", "event-access-test-secret")
	token, _, err := auth.GenerateToken(auth.DefaultConfig(), 42, "openid")
	if err != nil {
		t.Fatal(err)
	}
	secret, website := "protected-fixture", "https://example.com"
	event := &models.Event{ID: 7, Name: "event", ResourceURL: &secret, QQGroup: &secret, OfficialWebsite: &website}
	for _, tt := range []struct {
		name, header              string
		status, userStatus        int
		missing, dbError, allowed bool
	}{
		{name: "anonymous", status: 1},
		{name: "invalid token", header: "Bearer invalid", status: 1},
		{name: "unverified", header: "Bearer " + token, status: 0},
		{name: "rejected", header: "Bearer " + token, status: 2},
		{name: "reviewing", header: "Bearer " + token, status: 3},
		{name: "verified", header: "Bearer " + token, status: 1, allowed: true},
		{name: "banned", header: "Bearer " + token, status: 1, userStatus: models.UserStatusBanned},
		{name: "deleted user", header: "Bearer " + token, missing: true},
		{name: "database unavailable", header: "Bearer " + token, dbError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			u := resourceAccessUserRepo{user: &models.User{ID: 42, AuthStatus: &tt.status, UserStatus: tt.userStatus}}
			if tt.missing {
				u.user = nil
			}
			if tt.dbError {
				u.err = errors.New("offline")
			}
			repo := &repository.Repository{User: u, Event: resourceAccessEventRepo{event: event}}
			s := NewServer(repo, &service.Services{Event: service.NewEventService(repo)})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v2/events/7", nil)
			req.Header.Set("Authorization", tt.header)
			ctx := echo.New().NewContext(req, rec)
			ctx.SetParamNames("id")
			ctx.SetParamValues("7")
			if err := s.GetEvent(ctx); err != nil {
				t.Fatal(err)
			}
			if rec.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("user-specific resources must not enter a shared cache")
			}
			if tt.dbError {
				if rec.Code != 500 || strings.Contains(rec.Body.String(), secret) {
					t.Fatal(rec.Body.String())
				}
				return
			}
			if rec.Code != 200 {
				t.Fatal(rec.Body.String())
			}
			var body struct {
				Data struct {
					Event struct {
						ResourceURL     *string `json:"resourceUrl"`
						QQGroup         *string `json:"qqGroup"`
						OfficialWebsite *string `json:"officialWebsite"`
					}
				}
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if (body.Data.Event.ResourceURL != nil) != tt.allowed || (body.Data.Event.QQGroup != nil) != tt.allowed {
				t.Fatal("resource authorization mismatch")
			}
			if body.Data.Event.OfficialWebsite == nil || *body.Data.Event.OfficialWebsite != website {
				t.Fatal("public website changed")
			}
		})
	}
}
