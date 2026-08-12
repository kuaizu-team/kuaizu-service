package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestProjectViewersSecondPageUsesStableOffset(t *testing.T) {
	raw, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	repo := NewProjectViewLogRepository(sqlx.NewDb(raw, "sqlmock"))

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT vl.user_id\\)").WithArgs(42).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(21))
	mock.ExpectQuery(regexp.QuoteMeta("ORDER BY last_viewed_at DESC, vl.user_id DESC")+
		"[[:space:]]+LIMIT \\? OFFSET \\?").WithArgs(42, 20, 20).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "talent_profile_id", "nickname", "avatar_url", "auth_status", "collaboration_score", "last_viewed_at"}).
			AddRow(7, nil, "viewer", nil, 0, nil, time.Now()))

	viewers, total, err := repo.GetViewers(context.Background(), 42, 2, 20)
	require.NoError(t, err)
	require.Equal(t, 21, total)
	require.Len(t, viewers, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTalentViewersSecondPageUsesStableOffset(t *testing.T) {
	raw, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	repo := NewTalentViewLogRepository(sqlx.NewDb(raw, "sqlmock"))

	mock.ExpectQuery("SELECT COUNT\\(DISTINCT vl.user_id\\)").WithArgs(42).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(21))
	mock.ExpectQuery(regexp.QuoteMeta("ORDER BY last_viewed_at DESC, vl.user_id DESC")+
		"[[:space:]]+LIMIT \\? OFFSET \\?").WithArgs(42, 20, 20).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "nickname", "avatar_url", "last_viewed_at"}).
			AddRow(7, "viewer", nil, time.Now()))

	viewers, total, err := repo.GetViewers(context.Background(), 42, 2, 20)
	require.NoError(t, err)
	require.Equal(t, 21, total)
	require.Len(t, viewers, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}
