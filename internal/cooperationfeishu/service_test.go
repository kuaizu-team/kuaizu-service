package cooperationfeishu

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type settingRemote struct{ link, external string }

func (r settingRemote) Call(_ context.Context, method, _ string, _ any, out any) error {
	if method == "GET" {
		body, _ := json.Marshal(map[string]any{"permission_public": map[string]string{"link_share_entity": r.link, "external_access_entity": r.external}})
		return json.Unmarshal(body, out)
	}
	return nil
}

type ownerRemote struct {
	granted   bool
	grantedID string
}

func (r *ownerRemote) Call(_ context.Context, method, _ string, body, out any) error {
	if method == http.MethodPost {
		request := body.(map[string]any)
		r.grantedID = request["member_id"].(string)
		r.granted = true
		return nil
	}
	items := []map[string]string{}
	if r.granted {
		items = append(items, map[string]string{"member_id": r.grantedID, "member_type": "openid", "perm": "full_access"})
	}
	encoded, _ := json.Marshal(map[string]any{"items": items})
	return json.Unmarshal(encoded, out)
}

func TestOnlyConfiguredOwnerGetsFullAccess(t *testing.T) {
	remote := &ownerRemote{}
	s := &Service{client: remote, ownerOpenID: "ou_owner"}
	require.NoError(t, s.ensureOwner(context.Background(), "private-base"))
	require.Equal(t, "ou_owner", remote.grantedID)
}

func TestBaseAllowsTenantLinkAccessButNotExternalAccess(t *testing.T) {
	for _, tc := range []struct {
		link, external string
		allowed        bool
	}{
		{"tenant_readable", "closed", true}, {"closed", "closed", false}, {"tenant_readable", "open", false}, {"", "", false},
	} {
		s := &Service{client: settingRemote{tc.link, tc.external}}
		err := s.shareForTenant(context.Background(), "base-token")
		require.Equal(t, tc.allowed, err == nil)
	}
}

func TestApplicationValuesOnlyUseCapturedCardFields(t *testing.T) {
	card := `{"nickname":"同学","phone":"13800000000","skills":["Go","设计"],"unrelatedSecret":"must-not-export"}`
	values, err := (application{ID: 5, UserID: 7, AttemptNo: 2, Status: "pending", Profile: &card}).values()
	require.NoError(t, err)
	require.Equal(t, "13800000000", values["手机"])
	require.Equal(t, "Go、设计", values["技能"])
	require.NotContains(t, values, "unrelatedSecret")
}
