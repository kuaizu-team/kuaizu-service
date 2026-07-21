package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

func TestMarkFailedAndOrderPushIfNotCompletedUsesProtectedAtomicUpdates(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()

	capturedExec.Lock()
	capturedExec.query = ""
	capturedExec.args = nil
	capturedExec.Unlock()

	repo := NewSmsNoticeRepository(sqlx.NewDb(db, "capture_user_repo"))
	updated, err := repo.MarkFailedAndOrderPushIfNotCompleted(
		context.Background(), 10, 20, "message center unavailable", time.Now())
	if err != nil {
		t.Fatalf("MarkFailedAndOrderPushIfNotCompleted returned error: %v", err)
	}
	if !updated {
		t.Fatal("MarkFailedAndOrderPushIfNotCompleted returned updated=false")
	}

	capturedExec.Lock()
	query := normalizeSQL(capturedExec.query)
	capturedExec.Unlock()
	for _, want := range []string{
		"UPDATE `order` SET",
		"WHERE id=? AND (push_status IS NULL OR push_status<>'success')",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("order query = %q, want to contain %q", query, want)
		}
	}
}
