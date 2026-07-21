package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestBeginOrderPushRetryUsesAtomicFailedStateGuard(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	capturedExec.Lock()
	capturedExec.query = ""
	capturedExec.args = nil
	capturedExec.Unlock()

	repo := New(sqlx.NewDb(db, "capture_user_repo"))
	started, err := repo.BeginOrderPushRetry(context.Background(), 52)
	if err != nil {
		t.Fatalf("BeginOrderPushRetry returned error: %v", err)
	}
	if !started {
		t.Fatal("BeginOrderPushRetry should report one affected row")
	}

	capturedExec.Lock()
	query := normalizeSQL(capturedExec.query)
	args := append([]driver.NamedValue(nil), capturedExec.args...)
	capturedExec.Unlock()

	for _, want := range []string{
		"push_status='pending'",
		"push_retry_count=push_retry_count+1",
		"WHERE id=? AND push_status='failed' AND refund_status=0",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q: %s", want, query)
		}
	}
	if len(args) != 1 || args[0].Value != int64(52) {
		t.Fatalf("args = %#v, want order id 52", args)
	}
}
