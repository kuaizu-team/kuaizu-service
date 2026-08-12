package vo

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

func TestAdminAccountListItemOmitsPasswordByDefault(t *testing.T) {
	item := NewAdminUserAccountVO(&models.AdminUser{
		ID: 1, Username: "event-manager",
	})
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"password"`) {
		t.Fatalf("admin account JSON exposed password: %s", data)
	}
}
