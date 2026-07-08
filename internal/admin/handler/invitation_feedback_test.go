package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
)

func TestUpdateUserInvitationConversationStatusRejectsSchoolAdmin(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/admin/users/1001/invitation/conversation-status", strings.NewReader(`{
		"conversation_status": "in_progress"
	}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("adminRole", models.AdminRoleSchoolAdmin)
	ctx.SetParamNames("id")
	ctx.SetParamValues("1001")

	server := &AdminServer{}
	if err := server.UpdateUserInvitationConversationStatus(ctx); err != nil {
		t.Fatalf("UpdateUserInvitationConversationStatus returned error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
