package repository

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestContactAccessOwnerAnonymousAndUnavailable(t *testing.T) {
	allowed, err := CanViewContacts(context.Background(), nil, 7, 7)
	require.NoError(t, err)
	require.True(t, allowed)
	allowed, err = CanViewContacts(context.Background(), nil, 0, 7)
	require.NoError(t, err)
	require.False(t, allowed)
	allowed, err = CanViewContacts(context.Background(), nil, 7, 8)
	require.Error(t, err)
	require.False(t, allowed)
}
func TestContactAccessUsesServerIdentityAndFailsClosed(t *testing.T) {
	for _, fail := range []bool{false, true} {
		raw, mock, err := sqlmock.New()
		require.NoError(t, err)
		db := sqlx.NewDb(raw, "sqlmock")
		expected := mock.ExpectQuery("SELECT .*pa.status=1.*ob.status=4").
			WithArgs(7, 7, 8, 8, 8, 7, 7, 7, 7, 8, 8, 8, 7, 8, 8, 7)
		if fail {
			expected.WillReturnError(errors.New("database unavailable"))
		} else {
			expected.WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(false))
		}
		allowed, err := CanViewContacts(context.Background(), db, 7, 8)
		require.False(t, allowed)
		if fail {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
		require.NoError(t, mock.ExpectationsWereMet())
		raw.Close()
	}
}
