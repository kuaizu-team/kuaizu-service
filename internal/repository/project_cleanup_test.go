package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
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
	found := false
	for _, table := range projectRelationTables {
		if table == "sms_notice" {
			t.Fatal("project cleanup references obsolete table sms_notice")
		}
		if table == "olive_branch_sms_notice" {
			found = true
		}
	}
	if !found {
		t.Fatal("project cleanup must clear olive_branch_sms_notice")
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
