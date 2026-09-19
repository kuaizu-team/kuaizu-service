package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/stretchr/testify/require"
)

func TestPurgeDeletedProjectsBeforeSkipsWhenNoExpiredProjects(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	setCapturedQuery([]string{"id"}, nil)
	capturedExec.Lock()
	capturedExec.query = ""
	capturedExec.args = nil
	capturedExec.Unlock()

	repo := New(sqlx.NewDb(db, "capture_user_repo"))
	deleted, err := repo.PurgeDeletedProjectsBefore(context.Background(), time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("PurgeDeletedProjectsBefore returned error: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted = %d, want 0", deleted)
	}

	capturedExec.Lock()
	query := capturedExec.query
	capturedExec.Unlock()
	if query != "" {
		t.Fatalf("unexpected exec query when no expired projects: %s", query)
	}
}

func TestPurgeDeletedProjectsBeforeHardDeletesOnlyExpiredDeletingProjects(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	cutoff := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)
	setCapturedQuery([]string{"id"}, [][]driver.Value{{int64(10)}, {int64(11)}})
	capturedExec.Lock()
	capturedExec.query = ""
	capturedExec.args = nil
	capturedExec.Unlock()

	repo := New(sqlx.NewDb(db, "capture_user_repo"))
	deleted, err := repo.PurgeDeletedProjectsBefore(context.Background(), cutoff)
	if err != nil {
		t.Fatalf("PurgeDeletedProjectsBefore returned error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want captured RowsAffected 1", deleted)
	}

	capturedQuery.Lock()
	selectQuery := normalizeSQL(capturedQuery.query)
	selectArgs := append([]driver.NamedValue(nil), capturedQuery.args...)
	capturedQuery.Unlock()
	for _, want := range []string{"status = ?", "deleted_at IS NOT NULL", "deleted_at <= ?"} {
		if !strings.Contains(selectQuery, want) {
			t.Fatalf("select query missing %q: %s", want, selectQuery)
		}
	}
	if len(selectArgs) != 2 || selectArgs[0].Value != int64(models.ProjectStatusDeleting) || selectArgs[1].Value != cutoff {
		t.Fatalf("select args = %#v", selectArgs)
	}

	capturedExec.Lock()
	deleteQuery := normalizeSQL(capturedExec.query)
	deleteArgs := append([]driver.NamedValue(nil), capturedExec.args...)
	capturedExec.Unlock()
	for _, want := range []string{"DELETE FROM project", "id IN (?, ?)", "status = ?", "deleted_at IS NOT NULL", "deleted_at <= ?"} {
		if !strings.Contains(deleteQuery, want) {
			t.Fatalf("delete query missing %q: %s", want, deleteQuery)
		}
	}
	if len(deleteArgs) != 4 || deleteArgs[0].Value != int64(10) || deleteArgs[1].Value != int64(11) || deleteArgs[2].Value != int64(models.ProjectStatusDeleting) || deleteArgs[3].Value != cutoff {
		t.Fatalf("delete args = %#v", deleteArgs)
	}
}

func TestProjectCleanupUsesActualSMSNoticeTable(t *testing.T) {
	foundSMSNotice := false
	foundProjectImage := false
	for _, table := range projectRelationTables {
		if table == "sms_notice" {
			t.Fatal("project cleanup references obsolete table sms_notice")
		}
		if table == "olive_branch_sms_notice" {
			foundSMSNotice = true
		}
		if table == "project_image" {
			foundProjectImage = true
		}
	}
	if !foundSMSNotice {
		t.Fatal("project cleanup must clear olive_branch_sms_notice")
	}
	if !foundProjectImage {
		t.Fatal("project cleanup must clear project_image")
	}
}

func TestProjectCleanupClearsPeriodicRatingsBeforeMembers(t *testing.T) {
	indexes := make(map[string]int, len(projectRelationTables))
	for i, table := range projectRelationTables {
		indexes[table] = i
	}
	memberIndex, hasMembers := indexes["project_members"]
	if !hasMembers {
		t.Fatal("project cleanup must clear project_members")
	}
	for _, table := range []string{"project_member_rating", "project_member_score"} {
		index, ok := indexes[table]
		if !ok {
			t.Fatalf("project cleanup must clear %s", table)
		}
		if index >= memberIndex {
			t.Fatalf("project cleanup must clear %s before project_members", table)
		}
	}
}

func TestProjectCleanupClearsTimelineAfterMemberships(t *testing.T) {
	memberIndex, timelineIndex := -1, -1
	for i, table := range projectRelationTables {
		switch table {
		case "project_members":
			memberIndex = i
		case "project_member_timeline":
			timelineIndex = i
		}
	}
	require.GreaterOrEqual(t, memberIndex, 0)
	require.Greater(t, timelineIndex, memberIndex,
		"remove memberships before cleaning timeline rows to exclude concurrent writes")
}

func TestProjectPurgeTimelineCleanupIsAtomic(t *testing.T) {
	for _, single := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("single=%t/fail=%t", single, fail), func(t *testing.T) {
				raw, mock, err := sqlmock.New()
				require.NoError(t, err)
				defer raw.Close()
				repo := New(sqlx.NewDb(raw, "sqlmock"))
				cutoff := time.Now().Add(-7 * 24 * time.Hour)
				if single {
					mock.ExpectBegin()
					mock.ExpectQuery("SELECT id FROM project").WithArgs(42, models.ProjectStatusDeleting, cutoff).
						WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
				} else {
					mock.ExpectQuery("SELECT id FROM project").WithArgs(models.ProjectStatusDeleting, cutoff).
						WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
					mock.ExpectBegin()
				}
				mock.ExpectExec("UPDATE media_upload").WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec("UPDATE media_upload").WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec("DELETE pme FROM project_milestone_evidence").WithArgs(42).WillReturnResult(sqlmock.NewResult(0, 0))
				cleanupErr := errors.New("timeline cleanup failed")
				for _, table := range projectRelationTables {
					expected := mock.ExpectExec("DELETE FROM " + table + " WHERE project_id IN").WithArgs(42)
					if table == "project_member_timeline" && fail {
						expected.WillReturnError(cleanupErr)
						break
					}
					expected.WillReturnResult(sqlmock.NewResult(0, 1))
				}
				if fail {
					mock.ExpectRollback()
				} else {
					mock.ExpectExec("DELETE FROM interaction_dashboard_view_state").WithArgs(42).WillReturnResult(sqlmock.NewResult(0, 0))
					mock.ExpectExec("DELETE FROM olive_branch_record").WithArgs(42).WillReturnResult(sqlmock.NewResult(0, 0))
					mock.ExpectExec("DELETE FROM project").WithArgs(42, models.ProjectStatusDeleting, cutoff).WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectCommit()
				}
				var count int64
				if single {
					count, err = repo.PurgeDeletedProjectBefore(context.Background(), 42, cutoff)
				} else {
					count, err = repo.PurgeDeletedProjectsBefore(context.Background(), cutoff)
				}
				if fail {
					require.ErrorIs(t, err, cleanupErr)
					require.Zero(t, count)
				} else {
					require.NoError(t, err)
					require.EqualValues(t, 1, count)
				}
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}
