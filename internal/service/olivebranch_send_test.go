package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/require"
)

type oliveSendUserRepo struct {
	repository.UserRepo
	user        models.User
	quotaWrites int
}

func (r *oliveSendUserRepo) GetByID(_ context.Context, id int) (*models.User, error) {
	return &models.User{ID: id}, nil
}
func (r *oliveSendUserRepo) ResetDailyFreeBranchQuotaIfNeededTx(context.Context, *sqlx.Tx, int) error {
	return nil
}
func (r *oliveSendUserRepo) GetByIDForUpdateTx(context.Context, *sqlx.Tx, int) (*models.User, error) {
	return &r.user, nil
}
func (r *oliveSendUserRepo) UpdateQuotaTx(context.Context, *sqlx.Tx, *models.User) error {
	r.quotaWrites++
	return nil
}

type oliveSendProjectRepo struct{ repository.ProjectRepo }

func (*oliveSendProjectRepo) GetByID(context.Context, int) (*models.Project, error) {
	return &models.Project{ID: 10, CreatorID: 1, Name: "project"}, nil
}
func (*oliveSendProjectRepo) ListMembers(context.Context, int) ([]models.ProjectMember, error) {
	return nil, nil
}

type oliveSendRecordRepo struct {
	repository.OliveBranchRepo
	creates int
}

func (*oliveSendRecordRepo) GetByID(context.Context, int) (*models.OliveBranch, error) {
	return &models.OliveBranch{ID: 20, SenderID: 1, ReceiverID: 2, RelatedProjectID: 10, Status: models.OliveBranchStatusRejected}, nil
}
func (r *oliveSendRecordRepo) CreateTx(_ context.Context, _ *sqlx.Tx, ob *models.OliveBranch) error {
	r.creates++
	ob.ID = 30
	return nil
}

type oliveSendDeliveryRepo struct {
	repository.WxSubscribeDeliveryRepo
	attempts int
}

func (r *oliveSendDeliveryRepo) Create(context.Context, *models.WxSubscribeDelivery) (int64, error) {
	r.attempts++
	return 0, errors.New("notification unavailable in test")
}

func TestSendOliveBranchQuotaGuards(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		status                           int
		member, paid, resend, wantCharge bool
	}{
		{name: "new free invitation", status: -1, wantCharge: true},
		{name: "rejected invitation sends again", status: -1, paid: true, wantCharge: true},
		{name: "pending duplicate", status: models.OliveBranchStatusPending},
		{name: "discussing duplicate", status: models.OliveBranchStatusDiscussing},
		{name: "accepted duplicate", status: models.OliveBranchStatusAccepted},
		{name: "current team member", member: true},
		{name: "rejected explicit resend", status: -1, resend: true, wantCharge: true},
		{name: "concurrent resend sees committed pending", status: models.OliveBranchStatusPending, resend: true},
		{name: "member joined before resend lock", member: true, resend: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, dbMock, err := sqlmock.New()
			require.NoError(t, err)
			defer raw.Close()
			repo := repository.New(sqlx.NewDb(raw, "sqlmock"))
			used, paid := 0, 2
			if tc.paid {
				used = models.OliveBranchDailyFreeQuota
			}
			user := &oliveSendUserRepo{user: models.User{ID: 1, FreeBranchUsedToday: &used, OliveBranchCount: &paid}}
			record := &oliveSendRecordRepo{}
			delivery := &oliveSendDeliveryRepo{}
			repo.User, repo.Project, repo.OliveBranch, repo.WxSubscribeDelivery = user, &oliveSendProjectRepo{}, record, delivery
			svc := NewOliveBranchService(repo, NewMessageService(repo, nil))
			dbMock.ExpectBegin()
			dbMock.ExpectQuery("SELECT creator_id FROM project WHERE id=\\? FOR UPDATE").WithArgs(10).
				WillReturnRows(sqlmock.NewRows([]string{"creator_id"}).AddRow(1))
			members := sqlmock.NewRows([]string{"id"})
			if tc.member {
				members.AddRow(99)
			}
			dbMock.ExpectQuery("SELECT id FROM project_members .* FOR UPDATE").WithArgs(10, 2).WillReturnRows(members)
			if !tc.member {
				rows := sqlmock.NewRows([]string{"id", "sender_id", "receiver_id", "related_project_id", "status", "cost_type"})
				if tc.status >= 0 {
					rows.AddRow(20, 3, 2, 10, tc.status, 1)
				}
				resendID := 0
				if tc.resend {
					resendID = 20
				}
				dbMock.ExpectQuery("SELECT id, sender_id, receiver_id, related_project_id, status, cost_type.*FOR UPDATE").
					WithArgs(10, 2, 0, 4, 1, resendID, 1).WillReturnRows(rows)
			}
			if tc.wantCharge {
				if tc.resend {
					dbMock.ExpectExec("UPDATE olive_branch_record SET status").WithArgs(0, 1, 20).WillReturnResult(sqlmock.NewResult(0, 1))
				}
				dbMock.ExpectCommit()
			} else {
				dbMock.ExpectRollback()
			}
			var result *models.OliveBranch
			if tc.resend {
				result, err = svc.ResendOliveBranch(context.Background(), 1, 20)
			} else {
				result, err = svc.SendOliveBranch(context.Background(), 1, SendRequest{ReceiverID: 2, RelatedProjectID: 10})
			}
			if tc.member || (tc.resend && !tc.wantCharge) {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}
			if tc.wantCharge {
				require.Equal(t, 1, user.quotaWrites)
				require.Equal(t, 1, delivery.attempts)
				if tc.paid {
					require.Equal(t, 1, *user.user.OliveBranchCount)
					require.Equal(t, models.OliveBranchDailyFreeQuota, *user.user.FreeBranchUsedToday)
				} else {
					require.Equal(t, 1, *user.user.FreeBranchUsedToday)
					require.Equal(t, 2, *user.user.OliveBranchCount)
				}
			} else {
				require.Zero(t, user.quotaWrites)
				require.Zero(t, record.creates)
				require.Zero(t, delivery.attempts)
				require.Equal(t, 0, *user.user.FreeBranchUsedToday)
				require.Equal(t, 2, *user.user.OliveBranchCount)
			}
			require.NoError(t, dbMock.ExpectationsWereMet())
		})
	}
}
