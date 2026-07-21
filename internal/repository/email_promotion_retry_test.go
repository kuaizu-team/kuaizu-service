package repository

import (
	"context"
	"database/sql/driver"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

func TestGetRetryRecipientUserIDsPrefersSnapshot(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	setCapturedQuery([]string{"user_id"}, [][]driver.Value{{int64(11)}, {int64(12)}})

	repo := NewEmailPromotionRepository(sqlx.NewDb(db, "capture_user_repo"))
	userIDs, err := repo.GetRetryRecipientUserIDs(context.Background(), 77, 2)
	if err != nil {
		t.Fatalf("GetRetryRecipientUserIDs returned error: %v", err)
	}
	if len(userIDs) != 2 || userIDs[0] != 11 || userIDs[1] != 12 {
		t.Fatalf("userIDs = %#v, want original snapshot [11 12]", userIDs)
	}

	capturedQuery.Lock()
	queries := append([]string(nil), capturedQuery.queries...)
	capturedQuery.Unlock()
	if len(queries) != 1 || !strings.Contains(normalizeSQL(queries[0]), "FROM email_promotion_recipient WHERE promotion_id = ?") {
		t.Fatalf("queries = %#v, want snapshot query only", queries)
	}
}

func TestGetRetryRecipientUserIDsFallsBackToLegacyTasks(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	setCapturedQueryQueue(
		captureQueryResult{columns: []string{"user_id"}},
		captureQueryResult{columns: []string{"id"}, rows: [][]driver.Value{{int64(21)}, {int64(22)}}},
	)

	repo := NewEmailPromotionRepository(sqlx.NewDb(db, "capture_user_repo"))
	userIDs, err := repo.GetRetryRecipientUserIDs(context.Background(), 88, 3)
	if err != nil {
		t.Fatalf("GetRetryRecipientUserIDs returned error: %v", err)
	}
	if len(userIDs) != 2 || userIDs[0] != 21 || userIDs[1] != 22 {
		t.Fatalf("userIDs = %#v, want legacy task users [21 22]", userIDs)
	}

	capturedQuery.Lock()
	queries := append([]string(nil), capturedQuery.queries...)
	capturedQuery.Unlock()
	if len(queries) != 2 {
		t.Fatalf("queries = %#v, want snapshot then legacy query", queries)
	}
	legacyQuery := normalizeSQL(queries[1])
	for _, want := range []string{"SELECT MIN(u.id) AS id", "GROUP BY LOWER(TRIM(recipient_email))", "GROUP BY et.recipient_email, et.first_task_id", "ORDER BY et.first_task_id ASC"} {
		if !strings.Contains(legacyQuery, want) {
			t.Fatalf("legacy query missing %q: %s", want, legacyQuery)
		}
	}
}

func TestMarkEmailPromotionFailedProtectsCompletedStatus(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	capturedExec.Lock()
	capturedExec.query = ""
	capturedExec.args = nil
	capturedExec.Unlock()

	repo := NewEmailPromotionRepository(sqlx.NewDb(db, "capture_user_repo"))
	updated, err := repo.MarkFailedIfNotCompleted(context.Background(), 77, "network timeout", time.Now())
	if err != nil {
		t.Fatalf("MarkFailedIfNotCompleted returned error: %v", err)
	}
	if !updated {
		t.Fatal("MarkFailedIfNotCompleted should report one affected row")
	}

	capturedExec.Lock()
	query := normalizeSQL(capturedExec.query)
	capturedExec.Unlock()
	for _, want := range []string{"UPDATE email_promotion SET", "WHERE id=? AND (status IS NULL OR status<>?)"} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q: %s", want, query)
		}
	}
	if strings.Contains(query, "updated_at") {
		t.Fatalf("failure query references column absent from email_promotion schema: %s", query)
	}

	schemaBytes, err := os.ReadFile(filepath.Join("..", "..", "sql", "create_mysql.sql"))
	if err != nil {
		t.Fatalf("read create_mysql.sql: %v", err)
	}
	schema := string(schemaBytes)
	start := strings.Index(schema, "CREATE TABLE `email_promotion` (")
	if start < 0 {
		t.Fatal("email_promotion table definition not found")
	}
	end := strings.Index(schema[start:], ") ENGINE=")
	if end < 0 {
		t.Fatal("email_promotion table definition end not found")
	}
	tableDDL := schema[start : start+end]
	if strings.Contains(tableDDL, "`updated_at`") {
		t.Fatal("email_promotion schema unexpectedly contains updated_at; revisit failure SQL assertion")
	}
	for _, column := range []string{"`status`", "`error_message`", "`completed_at`"} {
		if !strings.Contains(tableDDL, column) {
			t.Fatalf("email_promotion schema missing required failure column %s", column)
		}
	}
}
