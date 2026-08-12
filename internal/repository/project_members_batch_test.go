package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestListMembersByProjectIDsEmptyInputDoesNotQuery(t *testing.T) {
	raw, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	repo := NewProjectRepository(sqlx.NewDb(raw, "sqlmock"))

	got, err := repo.ListMembersByProjectIDs(context.Background(), []int{0, -1})
	require.NoError(t, err)
	require.Empty(t, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListMembersByProjectIDsDeduplicatesInputAndGroupsSharedUsers(t *testing.T) {
	raw, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	repo := NewProjectRepository(sqlx.NewDb(raw, "sqlmock"))
	now := time.Now()

	mock.ExpectQuery("FROM project_members pm").WithArgs(2, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "user_id", "role", "created_at", "role_name"}).
			AddRow(20, 1, 7, "TEAM_MEMBER", now, "成员").
			AddRow(21, 2, 7, "TEAM_LEADER", now, "队长").
			AddRow(22, 2, 8, "TEAM_MEMBER", now, "成员"))
	mock.ExpectQuery("FROM `user` u").
		WillReturnRows(sqlmock.NewRows([]string{"id", "openid", "nickname", "avatar_url", "auth_status", "school_name", "major_name", "talent_profile_id"}).
			AddRow(7, "openid-7", "shared", nil, 1, nil, nil, nil).
			AddRow(8, "openid-8", "second", nil, 0, nil, nil, nil))

	got, err := repo.ListMembersByProjectIDs(context.Background(), []int{2, 1, 2, 0})
	require.NoError(t, err)
	require.Len(t, got[1], 1)
	require.Len(t, got[2], 2)
	require.Equal(t, 7, got[1][0].UserID)
	require.Equal(t, 7, got[2][0].UserID)
	require.Equal(t, "shared", *got[1][0].User.Nickname)
	require.Equal(t, "shared", *got[2][0].User.Nickname)
	require.NoError(t, mock.ExpectationsWereMet())
}
