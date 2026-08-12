package handler

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

func TestShouldIncludeUserOverviewPermissionIsolation(t *testing.T) {
	require.False(t, shouldIncludeUserOverview("false", models.AdminRoleSuperAdmin))
	require.False(t, shouldIncludeUserOverview("true", models.AdminRoleEventManager))
	require.True(t, shouldIncludeUserOverview("true", models.AdminRoleSuperAdmin))
	require.True(t, shouldIncludeUserOverview("true", models.AdminRoleSchoolSuperAdmin))
	require.True(t, shouldIncludeUserOverview("true", models.AdminRoleSchoolAdmin))
}

func TestWaitUserOverviewRejectsPartialDownstreamResult(t *testing.T) {
	result := &adminUserOverview{}
	var group errgroup.Group
	group.Go(func() error {
		result.Activity.ProjectsTotal = 3
		return nil
	})
	group.Go(func() error { return errors.New("ratings unavailable") })

	got, err := waitUserOverview(&group, result)
	require.EqualError(t, err, "ratings unavailable")
	require.Nil(t, got)
}

func TestAdminUserDetailOverviewResponseShape(t *testing.T) {
	payload, err := json.Marshal(&adminUserDetailResponse{Overview: &adminUserOverview{}})
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	overview, ok := decoded["overview"].(map[string]any)
	require.True(t, ok)
	for _, key := range []string{"activity", "smsSendCounts", "invitation", "ratings"} {
		require.Contains(t, overview, key)
	}
}
