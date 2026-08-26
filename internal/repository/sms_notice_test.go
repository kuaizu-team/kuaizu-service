package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestGetSmsNoticeByOrderSupportsNullableOliveBranchRecordID(t *testing.T) {
	if !strings.Contains(normalizeSQL(smsNoticeSelectColumns),
		"COALESCE(olive_branch_record_id, 0) AS olive_branch_record_id") {
		t.Fatal("sms notice select columns must normalize a nullable olive_branch_record_id")
	}

	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	defer rawDB.Close()

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "channel", "business_tag", "trace_id", "order_id", "olive_branch_record_id",
		"member_removal_id", "project_id", "sender_id", "receiver_id", "sms_content", "status",
		"error_message", "provider", "provider_biz_id", "started_at", "completed_at", "created_at", "updated_at",
	}).AddRow(20, "SMS", "project_application_sms_rejected", "PROJECT_APPLICATION_SMS:134", 134, 0,
		nil, 399, 1130, 1505, "PROJECT_APPLICATION_SMS:754:rejected", 3,
		"短信内容不支持", nil, nil, now, now, now, now)
	mock.ExpectQuery("SELECT").WithArgs(134).WillReturnRows(rows)

	repo := NewSmsNoticeRepository(sqlx.NewDb(rawDB, "sqlmock"))
	notice, err := repo.GetByOrderID(context.Background(), 134)
	if err != nil {
		t.Fatalf("GetByOrderID returned error: %v", err)
	}
	if notice == nil || notice.OliveBranchRecordID != 0 {
		t.Fatalf("GetByOrderID notice = %#v, want normalized olive branch id 0", notice)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

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
