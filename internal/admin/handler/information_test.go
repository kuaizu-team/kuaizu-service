package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
)

func TestAdminListInformationRequiresSuperAdmin(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/information", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("adminRole", models.AdminRoleSchoolSuperAdmin)

	server := &AdminServer{
		repo: &repository.Repository{InformationContent: &fakeAdminInformationRepo{}},
	}

	if err := server.ListInformation(ctx); err != nil {
		t.Fatalf("ListInformation returned error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAdminListInformationPassesCategoryAndReturnsArray(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/information?category=campus_event&page=2&size=20", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("adminRole", models.AdminRoleSuperAdmin)

	infoRepo := &fakeAdminInformationRepo{}
	server := &AdminServer{
		repo: &repository.Repository{InformationContent: infoRepo},
	}

	if err := server.ListInformation(ctx); err != nil {
		t.Fatalf("ListInformation returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if infoRepo.params.Category == nil || *infoRepo.params.Category != models.InformationCategoryCampusEvent {
		t.Fatalf("category = %v, want campus_event", infoRepo.params.Category)
	}
	if infoRepo.params.Page != 2 || infoRepo.params.Size != 20 {
		t.Fatalf("page/size = %d/%d, want 2/20", infoRepo.params.Page, infoRepo.params.Size)
	}

	var resp struct {
		Data struct {
			List []json.RawMessage `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Data.List == nil {
		t.Fatalf("list is nil, want empty array")
	}
}

func TestAdminListInformationRejectsInvalidCategory(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/information?category=bad", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("adminRole", models.AdminRoleSuperAdmin)

	server := &AdminServer{
		repo: &repository.Repository{InformationContent: &fakeAdminInformationRepo{}},
	}

	if err := server.ListInformation(ctx); err != nil {
		t.Fatalf("ListInformation returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

type fakeAdminInformationRepo struct {
	repository.InformationContentRepo

	params repository.InformationContentListParams
}

func (f *fakeAdminInformationRepo) AdminList(_ context.Context, params repository.InformationContentListParams) ([]models.InformationContent, int64, error) {
	f.params = params
	return []models.InformationContent{}, 0, nil
}
