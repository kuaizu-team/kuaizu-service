package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
)

func TestListInformationAllowsAllCategories(t *testing.T) {
	cases := []api.ListInformationParamsCategory{
		api.CampusEvent,
		api.CampusProject,
		api.KuaizuTalking,
		api.DeveloperWeekly,
	}

	for _, category := range cases {
		t.Run(string(category), func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/v2/information/list?category="+string(category), nil)
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)

			infoRepo := &fakeInformationContentRepo{}
			server := &Server{
				repo: &repository.Repository{InformationContent: infoRepo},
			}

			err := server.ListInformation(ctx, api.ListInformationParams{Category: category})
			if err != nil {
				t.Fatalf("ListInformation returned error: %v", err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if infoRepo.category != string(category) {
				t.Fatalf("category = %s, want %s", infoRepo.category, category)
			}
			if infoRepo.limit != 4 {
				t.Fatalf("limit = %d, want 4", infoRepo.limit)
			}

			var resp struct {
				Code    int               `json:"code"`
				Message string            `json:"message"`
				Data    []json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if resp.Code != 0 || resp.Message != "success" {
				t.Fatalf("response code/message = %d/%q, want 0/success", resp.Code, resp.Message)
			}
			if resp.Data == nil {
				t.Fatalf("data is nil, want empty array")
			}
		})
	}
}

func TestListInformationRejectsInvalidCategory(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/information/list?category=bad", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	server := &Server{
		repo: &repository.Repository{InformationContent: &fakeInformationContentRepo{}},
	}

	err := server.ListInformation(ctx, api.ListInformationParams{Category: "bad"})
	if err != nil {
		t.Fatalf("ListInformation returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

type fakeInformationContentRepo struct {
	repository.InformationContentRepo

	category string
	limit    int
}

func (f *fakeInformationContentRepo) ListPublishedByCategory(_ context.Context, category string, limit int) ([]models.InformationContent, error) {
	f.category = category
	f.limit = limit
	return []models.InformationContent{}, nil
}
