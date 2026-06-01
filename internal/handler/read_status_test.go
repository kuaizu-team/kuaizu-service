package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
)

func TestMarkReceiverOliveBranchReadAllowsEmptyBody(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/olive-branches/received/mark-read", strings.NewReader(""))
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("userID", 42)

	oliveRepo := &fakeOliveBranchRepo{}
	server := &Server{
		repo: &repository.Repository{OliveBranch: oliveRepo},
	}

	if err := server.MarkReceiverOliveBranchRead(ctx); err != nil {
		t.Fatalf("MarkReceiverOliveBranchRead returned error: %v", err)
	}
	if oliveRepo.receiverID != 42 {
		t.Fatalf("receiverID = %d, want 42", oliveRepo.receiverID)
	}
	if oliveRepo.ids != nil {
		t.Fatalf("ids = %v, want nil for empty body", oliveRepo.ids)
	}
}

func TestMarkReviewerApplicationReadPassesProjectAndIDs(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/project-applications/mark-read", strings.NewReader(`{"projectId":7,"ids":[1,2]}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("userID", 42)

	appRepo := &fakeApplicationRepo{}
	server := &Server{
		repo: &repository.Repository{Application: appRepo},
	}

	if err := server.MarkReviewerApplicationRead(ctx); err != nil {
		t.Fatalf("MarkReviewerApplicationRead returned error: %v", err)
	}
	if appRepo.projectID != 7 || appRepo.ownerID != 42 {
		t.Fatalf("projectID/ownerID = %d/%d, want 7/42", appRepo.projectID, appRepo.ownerID)
	}
	if len(appRepo.ids) != 2 || appRepo.ids[0] != 1 || appRepo.ids[1] != 2 {
		t.Fatalf("ids = %v, want [1 2]", appRepo.ids)
	}
}

type fakeApplicationRepo struct {
	repository.ApplicationRepo

	projectID int
	ownerID   int
	ids       []int
}

func (f *fakeApplicationRepo) MarkReviewerRead(_ context.Context, projectID, ownerID int, ids []int) error {
	f.projectID = projectID
	f.ownerID = ownerID
	f.ids = ids
	return nil
}

type fakeOliveBranchRepo struct {
	repository.OliveBranchRepo

	receiverID int
	ids        []int
}

func (f *fakeOliveBranchRepo) MarkReceiverRead(_ context.Context, receiverID int, ids []int) error {
	f.receiverID = receiverID
	f.ids = ids
	return nil
}
