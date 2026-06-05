package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
)

func TestInteractionParamsDefaultsAndValues(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/?page=2&size=25&days=7", nil)
	ctx := e.NewContext(req, httptest.NewRecorder())
	page, size, days := interactionParams(ctx)
	if page != 2 || size != 25 || days != 7 {
		t.Fatalf("unexpected params: %d %d %d", page, size, days)
	}
}

func TestFavoriteTalentCardResponseExcludesPrivateFields(t *testing.T) {
	phone, email, wechat := "13800000000", "private@example.com", "private-wechat"
	profile := models.TalentProfile{Phone: &phone, Email: &email, WechatID: &wechat}
	body, err := json.Marshal(profile.ToVO())
	if err != nil {
		t.Fatal(err)
	}
	for _, privateField := range []string{"phone", "email", "wechat"} {
		if strings.Contains(string(body), `"`+privateField+`"`) {
			t.Fatalf("favorite talent card leaked %s: %s", privateField, body)
		}
	}
}
