package service

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBatchTalentRejectsEntireUnauthorizedSelection(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	svc := &TalentProfileService{repo: repository.New(sqlx.NewDb(db, "sqlmock"))}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT u.id,u.school_id.*FOR UPDATE").WithArgs(1, 2).WillReturnRows(sqlmock.NewRows([]string{"id", "school_id", "profile_id", "status"}).AddRow(1, 10, 20, 2).AddRow(2, 99, 21, 2))
	mock.ExpectRollback()
	result, err := svc.BatchApproveUsers(context.Background(), []int{1, 2}, []int{10})
	require.Nil(t, result)
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, ErrCodeForbidden, serviceErr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchTalentSkipsReviewedAndMissingProfiles(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	svc := &TalentProfileService{repo: repository.New(sqlx.NewDb(db, "sqlmock"))}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT u.id,u.school_id.*FOR UPDATE").WithArgs(1, 2, 3).WillReturnRows(sqlmock.NewRows([]string{"id", "school_id", "profile_id", "status"}).AddRow(1, 10, 20, 1).AddRow(2, 11, 21, 0).AddRow(3, 10, nil, nil))
	mock.ExpectCommit()
	result, err := svc.BatchApproveUsers(context.Background(), []int{1, 2, 3}, []int{10, 11})
	require.NoError(t, err)
	require.Len(t, result, 3)
	for _, r := range result {
		require.False(t, r.Approved)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchTalentRollsBackOnUpdateFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	svc := &TalentProfileService{repo: repository.New(sqlx.NewDb(db, "sqlmock"))}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT u.id,u.school_id.*FOR UPDATE").WithArgs(1, 2).WillReturnRows(sqlmock.NewRows([]string{"id", "school_id", "profile_id", "status"}).AddRow(1, 10, 20, 2).AddRow(2, 10, 21, 2))
	mock.ExpectExec("UPDATE talent_profile SET").WithArgs(1, 20, 2).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE talent_profile SET").WithArgs(1, 21, 2).WillReturnError(errors.New("write failed"))
	mock.ExpectRollback()
	result, err := svc.BatchApproveUsers(context.Background(), []int{1, 2}, []int{10})
	require.Error(t, err)
	require.Nil(t, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchTalentEmptySchoolScopeCannotApprove(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	svc := &TalentProfileService{repo: repository.New(sqlx.NewDb(db, "sqlmock"))}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT u.id,u.school_id.*FOR UPDATE").WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"id", "school_id", "profile_id", "status"}).AddRow(1, 10, 20, 2))
	mock.ExpectRollback()
	_, err = svc.BatchApproveUsers(context.Background(), []int{1}, []int{})
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchTalentApprovesPendingAndNotifiesOnlyAfterCommit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	svc := &TalentProfileService{repo: repository.New(sqlx.NewDb(db, "sqlmock"))}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT u.id,u.school_id.*FOR UPDATE").WithArgs(1, 2).WillReturnRows(sqlmock.NewRows([]string{"id", "school_id", "profile_id", "status"}).AddRow(1, 10, 20, 2).AddRow(2, 11, 21, 1))
	mock.ExpectExec("UPDATE talent_profile SET").WithArgs(1, 20, 2).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	// The existing notifier reads the user after commit. A missing recipient remains
	// non-fatal, and the skipped user must never enter the notification path.
	mock.ExpectQuery("SELECT").WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	result, err := svc.BatchApproveUsers(context.Background(), []int{1, 2}, []int{10, 11})
	require.NoError(t, err)
	require.Len(t, result, 2)
	require.True(t, result[0].Approved)
	require.False(t, result[1].Approved)
	require.NoError(t, mock.ExpectationsWereMet())
}
