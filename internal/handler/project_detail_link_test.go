package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
	"github.com/labstack/echo/v4"
)

type capturingProjectURLGenerator struct {
	input wechat.URLLinkRequest
}

func (f *capturingProjectURLGenerator) GenerateURLLink(_ context.Context, input wechat.URLLinkRequest) (string, error) {
	f.input = input
	return "https://wxaurl.cn/project-ticket", nil
}

type handlerProjectLookup struct {
	projects map[int]*models.Project
}

func (s handlerProjectLookup) GetByID(_ context.Context, id int) (*models.Project, error) {
	return s.projects[id], nil
}

func TestOpenProjectDetailRedirectsToWeChatURLLink(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/open/project-detail?id=402&source=2", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	generator := &capturingProjectURLGenerator{}
	server := NewServer(nil, &service.Services{
		ProjectDetailLink: service.NewProjectDetailLinkService(generator, handlerProjectLookup{
			projects: map[int]*models.Project{402: {ID: 402}},
		}),
	})

	if err := server.OpenProjectDetail(ctx); err != nil {
		t.Fatalf("OpenProjectDetail: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if location := rec.Header().Get("Location"); location != "https://wxaurl.cn/project-ticket" {
		t.Fatalf("Location = %q", location)
	}
	if generator.input.Path != "/pages/project-detail/project-detail" || generator.input.Query != "id=402&source=2" {
		t.Fatalf("unexpected input: %#v", generator.input)
	}
	if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q", cacheControl)
	}
}

func TestOpenProjectDetailDefaultsSourceAndRejectsInvalidID(t *testing.T) {
	e := echo.New()
	generator := &capturingProjectURLGenerator{}
	server := NewServer(nil, &service.Services{
		ProjectDetailLink: service.NewProjectDetailLinkService(generator, handlerProjectLookup{
			projects: map[int]*models.Project{403: {ID: 403}},
		}),
	})

	validReq := httptest.NewRequest(http.MethodGet, "/api/v2/open/project-detail?id=403", nil)
	validRec := httptest.NewRecorder()
	if err := server.OpenProjectDetail(e.NewContext(validReq, validRec)); err != nil {
		t.Fatalf("default source: %v", err)
	}
	if generator.input.Query != "id=403&source=2" {
		t.Fatalf("default query = %q", generator.input.Query)
	}

	invalidReq := httptest.NewRequest(http.MethodGet, "/api/v2/open/project-detail?id=bad", nil)
	invalidRec := httptest.NewRecorder()
	if err := server.OpenProjectDetail(e.NewContext(invalidReq, invalidRec)); err != nil {
		t.Fatalf("invalid id response: %v", err)
	}
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d, want %d", invalidRec.Code, http.StatusBadRequest)
	}
}

func TestOpenProjectDetailReturnsNotFoundWithoutGeneratingLink(t *testing.T) {
	e := echo.New()
	generator := &capturingProjectURLGenerator{}
	server := NewServer(nil, &service.Services{
		ProjectDetailLink: service.NewProjectDetailLinkService(generator, handlerProjectLookup{projects: map[int]*models.Project{}}),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v2/open/project-detail?id=999&source=2", nil)
	rec := httptest.NewRecorder()

	if err := server.OpenProjectDetail(e.NewContext(req, rec)); err != nil {
		t.Fatalf("not found response: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if generator.input.Path != "" {
		t.Fatalf("generator should not be called: %#v", generator.input)
	}
}
