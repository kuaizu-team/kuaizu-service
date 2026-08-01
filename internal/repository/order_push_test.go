package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"
	"time"

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
		"push_retry_count < ?",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q: %s", want, query)
		}
	}
	if len(args) != 2 || args[0].Value != int64(52) || args[1].Value != int64(maxOrderPushRetries) {
		t.Fatalf("args = %#v, want order id 52 and retry limit %d", args, maxOrderPushRetries)
	}
}

func TestAutomaticDeliveryRecoveryExcludesFailedOrders(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	setCapturedQuery([]string{"id"}, nil)
	repo := New(sqlx.NewDb(db, "capture_user_repo"))

	if _, err := repo.ListRecoverableOrderDeliveries(context.Background(), time.Now(), 100); err != nil {
		t.Fatal(err)
	}
	capturedQuery.Lock()
	query := normalizeSQL(capturedQuery.query)
	capturedQuery.Unlock()
	if !strings.Contains(query, "push_status IS NULL OR (push_status='pending' AND updated_at < ?)") {
		t.Fatalf("recovery query does not limit candidates to unclaimed/stale pending: %s", query)
	}
	if strings.Contains(query, "'failed'") {
		t.Fatalf("automatic recovery must not include failed orders: %s", query)
	}
}

func TestInitialDeliveryClaimDoesNotReclaimFailedOrder(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	capturedExec.Lock()
	capturedExec.query = ""
	capturedExec.args = nil
	capturedExec.Unlock()
	repo := New(sqlx.NewDb(db, "capture_user_repo"))

	if _, err := repo.BeginOrderPushDeliveryForUser(context.Background(), 52, 7); err != nil {
		t.Fatal(err)
	}
	capturedExec.Lock()
	query := normalizeSQL(capturedExec.query)
	capturedExec.Unlock()
	if !strings.Contains(query, "AND push_status IS NULL") || strings.Contains(query, "'failed'") {
		t.Fatalf("initial claim must only claim an unclaimed order: %s", query)
	}
}

func TestReleaseDeliveryClaimOnlyReleasesOwnedPendingOrder(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	capturedExec.Lock()
	capturedExec.query = ""
	capturedExec.args = nil
	capturedExec.Unlock()
	repo := New(sqlx.NewDb(db, "capture_user_repo"))

	released, err := repo.ReleaseOrderPushDeliveryForUser(context.Background(), 52, 1130)
	if err != nil {
		t.Fatalf("ReleaseOrderPushDeliveryForUser returned error: %v", err)
	}
	if !released {
		t.Fatal("ReleaseOrderPushDeliveryForUser should report one affected row")
	}

	capturedExec.Lock()
	query := normalizeSQL(capturedExec.query)
	args := append([]driver.NamedValue(nil), capturedExec.args...)
	capturedExec.Unlock()
	for _, want := range []string{
		"push_status=NULL", "last_push_time=NULL",
		"WHERE id=? AND user_id=? AND status=? AND refund_status=0 AND push_status='pending'",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q: %s", want, query)
		}
	}
	if len(args) != 3 || args[0].Value != int64(52) || args[1].Value != int64(1130) || args[2].Value != int64(1) {
		t.Fatalf("args = %#v, want order id, owner id and paid status", args)
	}
}

func TestRecoveryClaimExcludesFailedOrders(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	capturedExec.Lock()
	capturedExec.query = ""
	capturedExec.args = nil
	capturedExec.Unlock()
	repo := New(sqlx.NewDb(db, "capture_user_repo"))

	if _, err := repo.ClaimRecoverableOrderDelivery(context.Background(), 52, time.Now()); err != nil {
		t.Fatal(err)
	}
	capturedExec.Lock()
	query := normalizeSQL(capturedExec.query)
	capturedExec.Unlock()
	if !strings.Contains(query, "push_status IS NULL OR (push_status='pending' AND updated_at < ?)") || strings.Contains(query, "'failed'") {
		t.Fatalf("recovery claim must exclude failed orders: %s", query)
	}
}

func TestUpdateOrderPushStatusForUserUsesOwnershipAndTerminalGuard(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	capturedExec.Lock()
	capturedExec.query = ""
	capturedExec.args = nil
	capturedExec.Unlock()

	repo := New(sqlx.NewDb(db, "capture_user_repo"))
	updated, err := repo.UpdateOrderPushStatusForUser(context.Background(), 52, 1130, "failed", nil)
	if err != nil {
		t.Fatalf("UpdateOrderPushStatusForUser returned error: %v", err)
	}
	if !updated {
		t.Fatal("UpdateOrderPushStatusForUser should report one affected row")
	}

	capturedExec.Lock()
	query := normalizeSQL(capturedExec.query)
	args := append([]driver.NamedValue(nil), capturedExec.args...)
	capturedExec.Unlock()

	for _, want := range []string{
		"WHERE id=? AND user_id=?",
		"push_status IS NULL OR push_status <> 'success' OR ? = 'success'",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query missing %q: %s", want, query)
		}
	}
	if len(args) != 5 {
		t.Fatalf("args = %#v, want five arguments", args)
	}
	if args[2].Value != int64(52) || args[3].Value != int64(1130) {
		t.Fatalf("args = %#v, want order id 52 and owner id 1130", args)
	}
}
