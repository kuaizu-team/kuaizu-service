package repository

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/stretchr/testify/require"
)

func TestConcurrentRegistrationReturnsOneStablePhoneConflict(t *testing.T) {
	raw, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	mock.MatchExpectationsInOrder(false)
	repo := NewUserRepository(sqlx.NewDb(raw, "sqlmock"))
	duplicate := &mysql.MySQLError{Number: 1062, Message: "Duplicate entry '13800138000' for key 'user.uq_user_phone'"}
	mock.ExpectExec("INSERT INTO `user`").WillReturnResult(sqlmock.NewResult(7, 1))
	mock.ExpectExec("INSERT INTO `user`").WillReturnError(duplicate)
	mock.ExpectQuery("FROM `user` u").WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"id", "openid", "phone", "auth_status", "created_at"}).
			AddRow(7, "openid-a", "13800138000", 0, time.Now()))

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, openid := range []string{"openid-a", "openid-b"} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			<-start
			_, err := repo.CreateWithPhone(context.Background(), value, "13800138000")
			errs <- err
		}(openid)
	}
	close(start)
	wg.Wait()
	close(errs)

	assertOneSuccessOneConflict(t, errs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestConcurrentEmailUpdateReturnsOneStableConflict(t *testing.T) {
	raw, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	mock.MatchExpectationsInOrder(false)
	repo := NewUserRepository(sqlx.NewDb(raw, "sqlmock"))
	duplicate := &mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'same@example.com' for key 'user.uq_user_email'"}
	mock.ExpectExec("UPDATE `user` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `user` SET").WillReturnError(duplicate)

	email := "same@example.com"
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []int{7, 8} {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			<-start
			errs <- repo.Update(context.Background(), &models.User{ID: userID, Email: &email})
		}(id)
	}
	close(start)
	wg.Wait()
	close(errs)

	assertOneSuccessOneContactConflict(t, errs, ErrUserEmailConflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestConcurrentPhoneUpdateReturnsOneStableConflict(t *testing.T) {
	raw, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	mock.MatchExpectationsInOrder(false)
	repo := NewUserRepository(sqlx.NewDb(raw, "sqlmock"))
	duplicate := &mysql.MySQLError{Number: 1062, Message: "Duplicate entry '13800138000' for key 'user.uq_user_phone'"}
	mock.ExpectExec("UPDATE `user` SET phone").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `user` SET phone").WillReturnError(duplicate)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []int{7, 8} {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()
			<-start
			errs <- repo.UpdatePhone(context.Background(), userID, "13800138000")
		}(id)
	}
	close(start)
	wg.Wait()
	close(errs)

	assertOneSuccessOneConflict(t, errs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func assertOneSuccessOneConflict(t *testing.T, errs <-chan error) {
	assertOneSuccessOneContactConflict(t, errs, ErrUserPhoneConflict)
}

func assertOneSuccessOneContactConflict(t *testing.T, errs <-chan error, conflict error) {
	t.Helper()
	var success, conflicts int
	for err := range errs {
		switch {
		case err == nil:
			success++
		case errors.Is(err, conflict):
			conflicts++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflicts)
}
