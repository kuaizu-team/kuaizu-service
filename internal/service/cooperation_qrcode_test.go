package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/stretchr/testify/require"
)

func TestQRCodeRequiresIndependentBackupAndCannotEnterTemplate(t *testing.T) {
	qr := models.CooperationContact{Type: "wechat_group", Value: models.CooperationQRPrefix + "test.png"}
	for _, contacts := range [][]models.CooperationContact{{qr}, {qr, qr}, {qr, {Type: "wechat", Value: ""}}} {
		in := CooperationContentInput{Title: "标题", Introduction: "说明", Contacts: contacts}
		require.Error(t, in.Validate())
	}
	in := CooperationContentInput{Title: "标题", Introduction: "说明", Contacts: []models.CooperationContact{qr, {Type: "wechat", Value: "backup"}}}
	require.NoError(t, in.Validate())
	in.Contacts[0].Value = "campus-cooperation-images/public.png"
	require.Error(t, in.Validate())
	order := CommercialOrderInput{Name: "商单", CompanyName: "公司", Template: []byte(`{"title":"标题","introduction":"说明","contacts":[{"type":"wechat_group","label":"群","value":"campus-cooperation-qrcodes/test.png"},{"type":"wechat","value":"backup"}]}`)}
	require.Error(t, order.Validate())
}

func TestQRCodeUploadDoesNotGrantMembersEditingPermission(t *testing.T) {
	s, m := cooperationTestService(t)
	m.ExpectQuery(`SELECT role,school_id,status FROM admin_user WHERE id=\?`).WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"role", "school_id", "status"}).AddRow(3, 42, 1))
	m.ExpectQuery(`(?s)SELECT c.id.*WHERE c.id=\?`).WithArgs(int64(9)).WillReturnRows(cooperationRows(42))
	// There is deliberately no file or OSS client: authorization must reject
	// the member before accessing either, even for their own school.
	result, err := s.UploadImage(context.Background(), 7, 9, nil, nil, "wechat_group")
	require.Error(t, err)
	require.Nil(t, result)
	require.NoError(t, m.ExpectationsWereMet())
}

func TestQRCodeExpiryIsServerOwnedAndCannotBeRenewedByResaving(t *testing.T) {
	for _, mode := range []string{"active", "expired", "foreign"} {
		t.Run(mode, func(t *testing.T) {
			s, m := cooperationTestService(t)
			m.ExpectBegin()
			tx, err := s.repo.DB().BeginTxx(context.Background(), nil)
			require.NoError(t, err)
			expiry := time.Now().Add(time.Hour)
			fake := time.Now().AddDate(5, 0, 0)
			if mode == "expired" {
				expiry = time.Now().Add(-time.Hour)
			}
			contacts := []models.CooperationContact{{Type: "wechat_group", Value: models.CooperationQRPrefix + "test.png", ExpiresAt: &fake, ImageURL: "https://untrusted.invalid/image"}}
			query := m.ExpectQuery(`SELECT expires_at FROM campus_cooperation_image WHERE object_key=\? AND cooperation_id=\? FOR UPDATE`).WithArgs(contacts[0].Value, int64(9))
			if mode == "foreign" {
				query.WillReturnRows(sqlmock.NewRows([]string{"expires_at"}))
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"expires_at"}).AddRow(expiry))
			}
			m.ExpectRollback()
			err = s.prepareQRContacts(context.Background(), tx, 9, contacts, nil)
			if mode == "active" {
				require.NoError(t, err)
				require.Equal(t, expiry, *contacts[0].ExpiresAt)
				require.Empty(t, contacts[0].ImageURL)
			} else {
				require.Error(t, err)
			}
			require.NoError(t, tx.Rollback())
			require.NoError(t, m.ExpectationsWereMet())
		})
	}
}

func TestExpiredQRCodeCanRemainAlongsideBackupAfterObjectCleanup(t *testing.T) {
	s, m := cooperationTestService(t)
	m.ExpectBegin()
	tx, err := s.repo.DB().BeginTxx(context.Background(), nil)
	require.NoError(t, err)
	expiry := time.Now().Add(-time.Hour)
	forged := time.Now().AddDate(1, 0, 0)
	old := []models.CooperationContact{{Type: "wechat_group", Value: models.CooperationQRPrefix + "old.png", ExpiresAt: &expiry}}
	contacts := []models.CooperationContact{{Type: "wechat_group", Value: old[0].Value, ExpiresAt: &forged}, {Type: "wechat", Value: "backup", ExpiresAt: &forged}}
	require.NoError(t, s.prepareQRContacts(context.Background(), tx, 9, contacts, old))
	require.Equal(t, expiry, *contacts[0].ExpiresAt)
	require.Nil(t, contacts[1].ExpiresAt)
	m.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, m.ExpectationsWereMet())
}

func TestReplacingQRCodeRevokesOldKeyBeforeDeletingIt(t *testing.T) {
	s, m := cooperationTestService(t)
	m.ExpectBegin()
	tx, err := s.repo.DB().BeginTxx(context.Background(), nil)
	require.NoError(t, err)
	old := []models.CooperationContact{{Type: "wechat_group", Value: models.CooperationQRPrefix + "old.png"}}
	expiry := time.Now().Add(time.Hour)
	contacts := []models.CooperationContact{{Type: "wechat_group", Value: models.CooperationQRPrefix + "new.png"}}
	m.ExpectQuery(`SELECT expires_at FROM campus_cooperation_image`).WithArgs(contacts[0].Value, int64(9)).WillReturnRows(sqlmock.NewRows([]string{"expires_at"}).AddRow(expiry))
	m.ExpectExec(`UPDATE campus_cooperation_image SET expires_at=LEAST`).WithArgs(old[0].Value, int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, s.prepareQRContacts(context.Background(), tx, 9, contacts, old))
	m.ExpectRollback()
	require.NoError(t, tx.Rollback())
	require.NoError(t, m.ExpectationsWereMet())
}

func TestQRCodeIsApprovalGatedAndExpiresWithoutRemovingBackup(t *testing.T) {
	for _, expired := range []bool{false, true} {
		expiry := time.Now().Add(time.Hour)
		if expired {
			expiry = time.Now().Add(-time.Hour)
		}
		c := &models.CampusCooperation{ContactsVisible: true, ApplicationRequired: true, Contacts: []models.CooperationContact{{Type: "wechat_group", Value: models.CooperationQRPrefix + "group.png", ExpiresAt: &expiry}, {Type: "wechat", Value: "backup"}}}
		for _, status := range []string{"", "pending", "rejected"} {
			require.Empty(t, cooperationPublic(c, status).Contacts)
		}
		public := cooperationPublic(c, "approved")
		require.Len(t, public.Contacts, 2)
		require.Empty(t, public.Contacts[0].Value)
		require.Equal(t, expired, public.Contacts[0].Expired)
		require.Equal(t, "backup", public.Contacts[1].Value)
		if expired {
			require.Empty(t, public.Contacts[0].ImageURL)
		} else {
			require.NotEmpty(t, public.Contacts[0].ImageURL)
		}
	}
}

func TestQRCodeCleanupIsBoundedAndRetainsFailedDeletesForRetry(t *testing.T) {
	s, m := cooperationTestService(t)
	first, second := models.CooperationQRPrefix+"failed.png", models.CooperationQRPrefix+"success.png"
	m.ExpectQuery(`SELECT object_key FROM campus_cooperation_image WHERE expires_at<=\? AND object_key LIKE \? ORDER BY expires_at,object_key LIMIT 100`).WithArgs(sqlmock.AnyArg(), models.CooperationQRPrefix+"%").WillReturnRows(sqlmock.NewRows([]string{"object_key"}).AddRow(first).AddRow(second))
	calls := []string{}
	s.deleteQRFile = func(ctx context.Context, key string) error {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), 3*time.Second)
		calls = append(calls, key)
		if key == first {
			return errors.New("temporary OSS failure")
		}
		return nil
	}
	m.ExpectExec(`DELETE FROM campus_cooperation_image WHERE object_key=\? AND expires_at<=\?`).WithArgs(second, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	s.cleanupExpiredQRCodes(context.Background())
	require.Equal(t, []string{first, second}, calls)
	require.NoError(t, m.ExpectationsWereMet())
}
