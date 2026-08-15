package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/auth"
	"github.com/labstack/echo/v4"
)

func TestGetProjectListViewerUserID(t *testing.T) {
	t.Setenv("JWT_SECRET", "project-list-viewer-test-secret")
	token, _, err := auth.GenerateToken(auth.DefaultConfig(), 42, "openid")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		header string
		want   int
	}{
		{name: "anonymous", want: 0},
		{name: "invalid token stays anonymous", header: "Bearer invalid", want: 0},
		{name: "valid optional token", header: "Bearer " + token, want: 42},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/v2/projects", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			ctx := e.NewContext(req, httptest.NewRecorder())
			if got := getProjectListViewerUserID(ctx); got != tt.want {
				t.Fatalf("viewer user ID = %d, want %d", got, tt.want)
			}
		})
	}
}
