package handler

import (
	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOversizedSearchReturnsBadRequest(t *testing.T) {
	keyword := strings.Repeat("中", 33)
	for _, target := range []string{"projects", "talent-profiles"} {
		rec := httptest.NewRecorder()
		ctx := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/"+target, nil), rec)
		s := &Server{}
		var err error
		if target == "projects" {
			err = s.ListProjects(ctx, api.ListProjectsParams{Keyword: &keyword})
		} else {
			err = s.ListTalentProfiles(ctx, api.ListTalentProfilesParams{Keyword: &keyword})
		}
		if err != nil || rec.Code != 400 {
			t.Fatalf("target=%s status=%d err=%v", target, rec.Code, err)
		}
	}
}
