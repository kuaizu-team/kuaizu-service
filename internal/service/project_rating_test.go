package service

import (
	"testing"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/stretchr/testify/require"
)

func TestProjectRoleRatingWeight(t *testing.T) {
	require.Equal(t, 1.0, projectRoleRatingWeight(models.ProjectRoleTeamLeader))
	require.Equal(t, 0.8, projectRoleRatingWeight("TECH_LEADER"))
	require.Equal(t, 0.6, projectRoleRatingWeight(models.ProjectRoleTeamMember))
	require.Equal(t, 0.4, projectRoleRatingWeight(models.ProjectRoleLearningMember))
}

func TestWeightedProjectRatingScoreUsesLatestRaterRows(t *testing.T) {
	score, count := weightedProjectRatingScore([]weightedRatingRow{
		{Score: 90, Weight: 1.0},
		{Score: 70, Weight: 0.6},
	})
	require.Equal(t, 2, count)
	require.Equal(t, 82.5, score)
}

func TestProjectRatingCooldown(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.Local)
	last := now.Add(-5 * 24 * time.Hour)
	canRate, days, next := projectRatingCooldown(&last, now)
	require.False(t, canRate)
	require.Equal(t, 25, days)
	require.Equal(t, last.Add(models.ProjectRatingCooldown), *next)

	expired := now.Add(-models.ProjectRatingCooldown)
	canRate, days, _ = projectRatingCooldown(&expired, now)
	require.True(t, canRate)
	require.Zero(t, days)
}
