package repository

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"testing"
	"time"
)

func TestSubscribeLostLeaseCannotDispatch(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	r := NewWxSubscribeDeliveryRepository(sqlx.NewDb(raw, "mysql"))
	mock.ExpectExec(`(?s)UPDATE wx_subscribe_delivery SET status=\?.*WHERE id=\? AND attempt_count=\? AND status=\?`).WithArgs("DISPATCHING", int64(1), 2, "PREPARING").WillReturnResult(sqlmock.NewResult(0, 0))
	ok, err := r.BeginDispatch(context.Background(), 1, 2)
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestSubscribeFinishFencesAttemptAndState(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	r := NewWxSubscribeDeliveryRepository(sqlx.NewDb(raw, "mysql"))
	mock.ExpectExec(`(?s)UPDATE wx_subscribe_delivery.*WHERE id = \? AND attempt_count = \? AND status IN \('PREPARING','DISPATCHING'\)`).WithArgs("SENT", "tpl", int64(1), 2).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := r.MarkSent(context.Background(), 1, 2, "tpl"); err == nil {
		t.Fatal("stale result must report lost claim")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestSubscribeStaleDispatchBecomesUnknownBeforeRecovery(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	r := NewWxSubscribeDeliveryRepository(sqlx.NewDb(raw, "mysql"))
	before := time.Now()
	mock.ExpectExec(`(?s)UPDATE wx_subscribe_delivery.*WHERE status IN \(\?,\?\) AND claimed_at<\?`).WithArgs("UNKNOWN", "DISPATCHING", "PROCESSING", before).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT id FROM wx_subscribe_delivery.*`).WithArgs("PENDING", "RETRY", "PREPARING", before, 100).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	ids, err := r.ListDue(context.Background(), before, 100)
	if err != nil || len(ids) != 0 {
		t.Fatalf("%v %v", ids, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
