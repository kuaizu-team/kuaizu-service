package service

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestProfilePrivacyAndAuthorizedContacts(t *testing.T) {
	for _, status := range []int{0, 1, 2} {
		for _, relation := range []bool{false, true} {
			raw, mock, err := sqlmock.New()
			require.NoError(t, err)
			repo := repository.New(sqlx.NewDb(raw, "sqlmock"))
			mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(relation))
			if !relation && status != 1 {
				mock.ExpectQuery("SELECT EXISTS").WithArgs(8, 7, 7).WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(false))
			}
			phone := "private-phone"
			profile := &models.TalentProfile{UserID: 8, Status: &status, Phone: &phone, Email: &phone, WechatID: &phone}
			err = (&TalentProfileService{repo: repo}).AuthorizeProfileRead(context.Background(), profile, 7)
			if !relation && status != 1 {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if relation {
					require.NotNil(t, profile.Phone)
				} else {
					require.Nil(t, profile.Phone)
					require.Nil(t, profile.Email)
					require.Nil(t, profile.WechatID)
				}
			}
			require.NoError(t, mock.ExpectationsWereMet())
			raw.Close()
		}
	}
	phone := "owner-phone"
	profile := &models.TalentProfile{UserID: 7, Phone: &phone}
	require.NoError(t, (&TalentProfileService{repo: &repository.Repository{}}).AuthorizeProfileRead(context.Background(), profile, 7))
	require.NotNil(t, profile.Phone)
}

func TestPendingReviewerCanReadPrivateProfileWithoutContacts(t *testing.T) {
	raw, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	repo := repository.New(sqlx.NewDb(raw, "mysql"))
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(false))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(8, 7, 7).WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(true))
	status := 0
	secret := "sensitive"
	profile := &models.TalentProfile{UserID: 8, Status: &status, Phone: &secret, Email: &secret, WechatID: &secret}
	require.NoError(t, (&TalentProfileService{repo: repo}).AuthorizeProfileRead(context.Background(), profile, 7))
	require.Nil(t, profile.Phone)
	require.Nil(t, profile.Email)
	require.Nil(t, profile.WechatID)
	require.NoError(t, mock.ExpectationsWereMet())
}
