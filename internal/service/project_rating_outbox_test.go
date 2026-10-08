package service

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRatingCalculationAndNotificationCommitTogether(t *testing.T) {
	for _, scenario := range []string{"changed", "unchanged", "outbox failed"} {
		t.Run(scenario, func(t *testing.T) {
			raw, dbMock, err := sqlmock.New()
			require.NoError(t, err)
			defer raw.Close()
			repo := repository.New(sqlx.NewDb(raw, "sqlmock"))
			repo.WxSubscribeDelivery = &oliveSendDeliveryRepo{}
			dbMock.ExpectBegin()
			joined := time.Now().Add(-10 * 24 * time.Hour)
			dbMock.ExpectQuery("SELECT id,user_id,role,created_at FROM project_members").
				WithArgs(10, 1, 7).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "role", "created_at"}).
				AddRow(11, 1, models.ProjectRoleTeamLeader, joined).AddRow(17, 7, models.ProjectRoleTeamMember, joined))
			dbMock.ExpectQuery("SELECT created_at FROM project_member_rating").WithArgs(10, 11, 17).
				WillReturnRows(sqlmock.NewRows([]string{"created_at"}))
			dbMock.ExpectExec("INSERT INTO project_member_rating").WillReturnResult(sqlmock.NewResult(5, 1))
			dbMock.ExpectQuery("(?s)SELECT r.score,r.rater_weight").WithArgs(17).
				WillReturnRows(sqlmock.NewRows([]string{"score", "rater_weight"}).AddRow(80, 1.0))
			dbMock.ExpectExec("INSERT INTO project_member_score").WillReturnResult(sqlmock.NewResult(0, 1))
			dbMock.ExpectQuery("SELECT COALESCE.*FOR UPDATE").WithArgs(7).
				WillReturnRows(sqlmock.NewRows([]string{"score"}).AddRow(90))
			dbMock.ExpectExec("UPDATE .*user.* SET collaboration_score").WithArgs(7, 7, 7).WillReturnResult(sqlmock.NewResult(0, 1))
			score := 80
			if scenario == "unchanged" {
				score = 90
			}
			dbMock.ExpectQuery("SELECT COALESCE").WithArgs(7).
				WillReturnRows(sqlmock.NewRows([]string{"score"}).AddRow(score))
			if scenario != "unchanged" {
				if scenario == "outbox failed" {
					dbMock.ExpectExec("INSERT INTO wx_subscribe_delivery").WillReturnError(errors.New("outbox down"))
				} else {
					dbMock.ExpectExec("INSERT INTO wx_subscribe_delivery").WillReturnResult(sqlmock.NewResult(9, 1))
				}
			}
			if scenario == "outbox failed" {
				dbMock.ExpectRollback()
			} else {
				dbMock.ExpectCommit()
			}
			result, err := NewProjectService(repo, nil, NewMessageService(repo, nil)).RateProjectMember(context.Background(), 10, 1, 7, 80)
			if scenario == "outbox failed" {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.Equal(t, 80.0, result.Score)
			}
			require.NoError(t, dbMock.ExpectationsWereMet())
		})
	}
}
