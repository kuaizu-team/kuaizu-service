package service

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPromotionRequiresSourceConsentAndProtectsNationalPublication(t *testing.T) {
	school := 42
	for _, tc := range []struct {
		name                string
		role                int
		schools             []int
		state, action, want string
	}{
		{"owner submits", 2, []int{42}, "local", "submit", "submitted"},
		{"owner withdraws before publication", 2, []int{42}, "submitted", "withdraw", "local"},
		{"platform promotes approved proposal", 1, nil, "submitted", "promote", "national"},
		{"platform cannot invent consent", 1, nil, "local", "submit", ""},
		{"platform cannot skip consent", 1, nil, "local", "promote", ""},
		{"other school cannot consent", 2, []int{88}, "local", "submit", ""},
		{"member cannot consent", 3, []int{42}, "local", "submit", ""},
		{"owner cannot promote", 2, []int{42}, "submitted", "promote", ""},
		{"national cannot be withdrawn", 2, []int{42}, "national", "withdraw", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := promotionTransition(repository.CooperationAdminScope{Role: tc.role, SchoolIDs: tc.schools}, orderOrigin{SchoolID: &school, Promotion: tc.state, Status: "available"}, tc.action)
			if tc.want == "" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
		})
	}
}

func TestSourceConsentIsRecordedUnderOrderLock(t *testing.T) {
	s, m := cooperationTestService(t)
	m.ExpectBegin()
	expectCooperationAdmin(m, 2, []int{42})
	m.ExpectQuery(`SELECT origin_school_id,promotion_status,status FROM commercial_order WHERE id=\? FOR UPDATE`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"origin_school_id", "promotion_status", "status"}).AddRow(42, "local", "available"))
	m.ExpectExec(`UPDATE commercial_order SET promotion_status='submitted',promotion_consent_by=\?,promotion_consent_at=NOW\(\),updated_by=\? WHERE id=\?`).WithArgs(7, 7, int64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	_, err := s.SaveOrder(context.Background(), 7, 3, CommercialOrderInput{Action: "submit"})
	require.NoError(t, err)
	require.NoError(t, m.ExpectationsWereMet())
}

func TestSchoolCreationAndAutomaticClaimAreAtomic(t *testing.T) {
	for _, failClaim := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback on claim failure"}[failClaim], func(t *testing.T) {
			s, m := cooperationTestService(t)
			m.ExpectBegin()
			expectCooperationAdmin(m, 2, []int{42})
			m.ExpectExec(`INSERT INTO commercial_order.*'local'`).WithArgs("商单", "公司", nil, "available", nil, 7, 7, 42).WillReturnResult(sqlmock.NewResult(3, 1))
			claim := m.ExpectExec(`INSERT INTO campus_cooperation`).WithArgs(int64(3), 42, 7)
			if failClaim {
				claim.WillReturnError(errors.New("claim failed"))
				m.ExpectRollback()
			} else {
				claim.WillReturnResult(sqlmock.NewResult(9, 1))
				m.ExpectCommit()
			}
			id, err := s.SaveOrder(context.Background(), 7, 0, CommercialOrderInput{Name: "商单", CompanyName: "公司", SchoolID: 42})
			if failClaim {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, int64(3), id)
			}
			require.NoError(t, m.ExpectationsWereMet())
		})
	}
}

func TestLocalOrderCannotBeClaimedByAnotherSchool(t *testing.T) {
	s, m := cooperationTestService(t)
	m.ExpectBegin()
	expectCooperationAdmin(m, 2, []int{88})
	m.ExpectQuery(`SELECT status,origin_school_id,promotion_status FROM commercial_order`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"status", "origin_school_id", "promotion_status"}).AddRow("available", 42, "local"))
	m.ExpectRollback()
	_, err := s.Claim(context.Background(), 7, 88, 3)
	require.Error(t, err)
	require.NoError(t, m.ExpectationsWereMet())
}

func TestSourceCannotEditGloballyPublishedCommercialOrder(t *testing.T) {
	s, m := cooperationTestService(t)
	m.ExpectBegin()
	expectCooperationAdmin(m, 2, []int{42})
	m.ExpectQuery(`SELECT origin_school_id,promotion_status,status FROM commercial_order`).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"origin_school_id", "promotion_status", "status"}).AddRow(42, "national", "available"))
	m.ExpectRollback()
	_, err := s.SaveOrder(context.Background(), 7, 3, CommercialOrderInput{Name: "修改", CompanyName: "公司"})
	require.Error(t, err)
	require.NoError(t, m.ExpectationsWereMet())
}
