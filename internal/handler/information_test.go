package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestListInformationReturnsDisplayOrder(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/information/list?category=campus_event", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)

	createdAt := time.Date(2026, 6, 2, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	server := &Server{
		repo: &repository.Repository{InformationContent: &fakeInformationContentRepo{
			items: []models.InformationContent{
				{
					ID:           1,
					Title:        "标题",
					URL:          "https://example.com",
					Content:      "摘要",
					DisplayOrder: 100,
					CreatedAt:    createdAt,
				},
			},
		}},
	}

	err := server.ListInformation(ctx, api.ListInformationParams{Category: api.CampusEvent})
	if err != nil {
		t.Fatalf("ListInformation returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		Data []struct {
			ID           int       `json:"id"`
			Title        string    `json:"title"`
			URL          string    `json:"url"`
			Content      string    `json:"content"`
			DisplayOrder int       `json:"display_order"`
			CreatedAt    time.Time `json:"created_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("data length = %d, want 1", len(resp.Data))
	}
	item := resp.Data[0]
	if item.ID != 1 || item.Title != "标题" || item.URL != "https://example.com" || item.Content != "摘要" {
		t.Fatalf("response item = %+v, want populated information item", item)
	}
	if item.DisplayOrder != 100 {
		t.Fatalf("display_order = %d, want 100", item.DisplayOrder)
	}
	if !item.CreatedAt.Equal(createdAt) {
		t.Fatalf("created_at = %s, want %s", item.CreatedAt, createdAt)
	}
}

type fakeInformationContentRepo struct {
	repository.InformationContentRepo

	category string
	limit    int
	items    []models.InformationContent
}

func (f *fakeInformationContentRepo) ListPublishedByCategory(_ context.Context, category string, limit int) ([]models.InformationContent, error) {
	f.category = category
	f.limit = limit
	if f.items != nil {
		return f.items, nil
	}
	return []models.InformationContent{}, nil
}
