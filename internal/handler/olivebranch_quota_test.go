package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
)

func TestGetMyOliveBranchQuotaResetsYesterdayUsage(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/olive-branch-quota", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("userID", 42)

	used := 5
	paid := 0
	yesterday := time.Now().AddDate(0, 0, -1)
	userRepo := &fakeQuotaUserRepo{
		user: &models.User{
			ID:                  42,
			OpenID:              "openid",
			FreeBranchUsedToday: &used,
			OliveBranchCount:    &paid,
			LastActiveDate:      &yesterday,
		},
	}
	server := &Server{
		repo: &repository.Repository{User: userRepo},
	}

	if err := server.GetMyOliveBranchQuota(ctx); err != nil {
		t.Fatalf("GetMyOliveBranchQuota returned error: %v", err)
	}
	if !userRepo.resetCalled {
		t.Fatal("ResetDailyFreeBranchQuotaIfNeeded was not called")
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			FreeBranchUsedToday int `json:"freeBranchUsedToday"`
			FreeRemaining       int `json:"freeRemaining"`
			PaidBalance         int `json:"paidBalance"`
			TotalRemaining      int `json:"totalRemaining"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Code != 200 {
		t.Fatalf("code = %d, want 200; body=%s", resp.Code, rec.Body.String())
	}
	if resp.Data.FreeBranchUsedToday != 0 || resp.Data.FreeRemaining != 5 || resp.Data.PaidBalance != 0 || resp.Data.TotalRemaining != 5 {
		t.Fatalf("quota = %+v, want used=0 freeRemaining=5 paid=0 total=5", resp.Data)
	}
}

type fakeQuotaUserRepo struct {
	repository.UserRepo

	user        *models.User
	resetCalled bool
}

func (f *fakeQuotaUserRepo) ResetDailyFreeBranchQuotaIfNeeded(_ context.Context, userID int) error {
	f.resetCalled = true
	if f.user != nil && f.user.ID == userID {
		zero := 0
		today := time.Now()
		f.user.FreeBranchUsedToday = &zero
		f.user.LastActiveDate = &today
	}
	return nil
}

func (f *fakeQuotaUserRepo) GetByID(_ context.Context, id int) (*models.User, error) {
	if f.user != nil && f.user.ID == id {
		return f.user, nil
	}
	return nil, nil
}
