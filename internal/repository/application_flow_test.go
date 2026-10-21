package repository

import (
	"context"
	"database/sql/driver"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

func TestApplicationDashboardStatsCountJoinedAsApproved(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewApplicationRepository(sqlx.NewDb(db, "capture_user_repo"))
	setCapturedQuery([]string{"total", "read_count", "approved", "processed"}, [][]driver.Value{{int64(1), int64(0), int64(1), int64(1)}})

	if _, err := repo.GetProjectDashboardStats(context.Background(), 42); err != nil {
		t.Fatal(err)
	}

	capturedQuery.Lock()
	args := append([]driver.NamedValue(nil), capturedQuery.args...)
	capturedQuery.Unlock()
	if len(args) != 3 || args[0].Value != int64(models.ApplicationStatusJoined) || args[1].Value != int64(models.ApplicationStatusPending) || args[2].Value != int64(42) {
		t.Fatalf("unexpected project dashboard args: %#v", args)
	}

	setCapturedQuery([]string{"total", "read_count", "approved", "processed"}, [][]driver.Value{{int64(1), int64(0), int64(1), int64(1)}})
	if _, err := repo.GetUserDashboardStats(context.Background(), 7); err != nil {
		t.Fatal(err)
	}

	capturedQuery.Lock()
	args = append([]driver.NamedValue(nil), capturedQuery.args...)
	capturedQuery.Unlock()
	if len(args) != 3 || args[0].Value != int64(models.ApplicationStatusJoined) || args[1].Value != int64(models.ApplicationStatusPending) || args[2].Value != int64(7) {
		t.Fatalf("unexpected user dashboard args: %#v", args)
	}
}

func TestCompleteRecruitRejectsPendingAndDiscussingApplications(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	repo := NewProjectRepository(sqlx.NewDb(raw, "sqlmock"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE project SET status").WithArgs(models.ProjectStatusRecruitCompleted, 42).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("(?s)UPDATE project_application.*status IN").WithArgs(models.ApplicationStatusRejected, 42, models.ApplicationStatusPending, models.ApplicationStatusDiscussing).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("INSERT INTO user_status_message").WithArgs("recruit-completed", 42).WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectCommit()
	count, err := repo.CompleteRecruit(context.Background(), 42)
	if err != nil || count != 3 {
		t.Fatalf("count=%d error=%v", count, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
