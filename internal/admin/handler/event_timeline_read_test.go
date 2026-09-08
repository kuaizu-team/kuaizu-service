package handler

import (
	"context"
	"errors"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type timelineReadRepo struct {
	repository.EventRepo
	err error
}

func (r timelineReadRepo) GetByIDWithProjectSchoolIDs(context.Context, int, []int) (*models.Event, error) {
	return &models.Event{ID: 7, Name: "event"}, nil
}
func (r timelineReadRepo) ListTimelineNodes(context.Context, int) ([]models.EventTimelineNode, error) {
	return nil, r.err
}
func TestAdminEventTimelineFailureIsNotAnEditableEmptyList(t *testing.T) {
	for _, fail := range []bool{false, true} {
		repo := timelineReadRepo{}
		if fail {
			repo.err = errors.New("offline")
		}
		s := NewAdminServer(&repository.Repository{Event: repo}, nil)
		rec := httptest.NewRecorder()
		ctx := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/events/7", nil), rec)
		ctx.Set("adminRole", models.AdminRoleSuperAdmin)
		ctx.SetParamNames("id")
		ctx.SetParamValues("7")
		if err := s.GetEvent(ctx); err != nil {
			t.Fatal(err)
		}
		if fail {
			if rec.Code != 500 || strings.Contains(rec.Body.String(), `"timeline":[]`) {
				t.Fatal(rec.Body.String())
			}
		} else {
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"timeline":[]`) {
				t.Fatal(rec.Body.String())
			}
		}
	}
}
