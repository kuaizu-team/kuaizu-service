package repository

import (
	"context"
	"database/sql/driver"
	"strings"
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
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewProjectRepository(sqlx.NewDb(db, "capture_user_repo"))
	capturedExec.Lock()
	capturedExec.query = ""
	capturedExec.args = nil
	capturedExec.Unlock()

	if _, err := repo.CompleteRecruit(context.Background(), 42); err != nil {
		t.Fatal(err)
	}

	capturedExec.Lock()
	query := normalizeSQL(capturedExec.query)
	args := append([]driver.NamedValue(nil), capturedExec.args...)
	capturedExec.Unlock()
	if !strings.Contains(query, "status IN (?, ?)") {
		t.Fatalf("query = %q, want status IN (?, ?)", query)
	}
	if len(args) != 4 || args[0].Value != int64(models.ApplicationStatusRejected) || args[1].Value != int64(42) || args[2].Value != int64(models.ApplicationStatusPending) || args[3].Value != int64(models.ApplicationStatusDiscussing) {
		t.Fatalf("unexpected complete recruit args: %#v", args)
	}
}
