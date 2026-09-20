package db

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestCheckProjectTimelineSchema(t *testing.T) {
	queries := []string{
		"SELECT default_timeline_hidden FROM project LIMIT 0",
		"SELECT id, project_id, user_id, title, detail, related_members, created_at, updated_at FROM project_member_timeline LIMIT 0",
	}
	for _, tc := range []struct {
		name      string
		failIndex int
		migration string
	}{
		{"ready", -1, ""},
		{"missing visibility column", 0, "migration_project_default_timeline_hidden.sql"},
		{"missing timeline table or columns", 1, "migration_project_member_timeline.sql"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer raw.Close()
			failure := errors.New("schema unavailable")
			for i, query := range queries {
				expected := mock.ExpectQuery(regexp.QuoteMeta(query))
				if i == tc.failIndex {
					expected.WillReturnError(failure)
					break
				}
				expected.WillReturnRows(sqlmock.NewRows([]string{"unused"})).RowsWillBeClosed()
			}
			err = CheckProjectTimelineSchema(context.Background(), sqlx.NewDb(raw, "sqlmock"))
			if tc.failIndex < 0 {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, failure)
				require.ErrorContains(t, err, tc.migration)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
