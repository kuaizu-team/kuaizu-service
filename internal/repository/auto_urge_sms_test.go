package repository

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestAutoUrgeCandidateScopeAndThresholds(t *testing.T) {
	// Inspect the real query contract as well as binding the keyset arguments.
	for _, want := range []string{"status=0", "WHERE pa.status=0", "UNION ALL", "UNION\n", "COUNT(*)>=3 OR MIN(pending_at)<=DATE_SUB(NOW(),INTERVAL 168 HOUR)", "pm.user_id"} {
		if !strings.Contains(autoUrgePendingCTE, want) {
			t.Fatalf("missing %s", want)
		}
	}
	if strings.Contains(autoUrgePendingCTE, "is_read") {
		t.Fatal("read must not mean processed")
	}
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	r := NewAutoUrgeRepository(sqlx.NewDb(raw, "mysql"))
	mock.ExpectQuery(regexp.QuoteMeta(autoUrgePendingCTE)+`(?s).*a.state IN \('ready','failed'\).*ORDER BY e.user_id LIMIT \?`).WithArgs(5, 200).WillReturnRows(sqlmock.NewRows([]string{"user_id", "nickname", "pending_count", "oldest_pending_at"}).AddRow(7, "测试用户", 3, time.Now()))
	rows, err := r.Candidates(context.Background(), 5, 200)
	if err != nil || len(rows) != 1 {
		t.Fatalf("%v %v", rows, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
func TestAutoUrgeClaimCompareAndSet(t *testing.T) {
	for _, affected := range []int64{0, 1} {
		raw, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		r := NewAutoUrgeRepository(sqlx.NewDb(raw, "mysql"))
		mock.ExpectExec(`INSERT INTO auto_urge_sms_state`).WithArgs(7).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`(?s)UPDATE auto_urge_sms_state.*WHERE user_id=\? AND state IN \('ready','failed'\).*next_retry_at<=NOW\(\)`).WithArgs("token", 7).WillReturnResult(sqlmock.NewResult(0, affected))
		got, err := r.Claim(context.Background(), 7, "token")
		if err != nil || got != (affected == 1) {
			t.Fatalf("%v %v", got, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		raw.Close()
	}
}
func TestAutoUrgeSnapshotAllowsUnchangedDataButRequiresClaim(t *testing.T) {
	for _, owns := range []bool{true, false} {
		raw, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		r := NewAutoUrgeRepository(sqlx.NewDb(raw, "mysql"))
		mock.ExpectExec(`UPDATE auto_urge_sms_state SET pending_count`).WithArgs(3, nil, 7, "token").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(`SELECT EXISTS`).WithArgs(7, "token").WillReturnRows(sqlmock.NewRows([]string{"owns"}).AddRow(owns))
		err = r.Snapshot(context.Background(), AutoUrgeCandidate{UserID: 7, PendingCount: 3}, "token")
		if (err == nil) != owns {
			t.Fatalf("owns=%v error=%v", owns, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		raw.Close()
	}
}
func TestAutoUrgeFinishIsClaimScoped(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	r := NewAutoUrgeRepository(sqlx.NewDb(raw, "mysql"))
	mock.ExpectExec(`(?s)UPDATE auto_urge_sms_state.*next_retry_at=CASE.*sent_at=CASE.*WHERE user_id=\? AND state='claimed' AND claim_token=\?`).WithArgs("sent", nil, "", "", "sent", "sent", 7, "token").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := r.Finish(context.Background(), 7, "token", AutoUrgeResult{State: "sent"}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
