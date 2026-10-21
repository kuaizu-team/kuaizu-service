package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestMessageOwnershipAndIdempotentRead(t *testing.T) {
	for _, missing := range []bool{true, false} {
		raw, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		repo := NewStatusNotificationRepository(sqlx.NewDb(raw, "sqlmock"))
		expected := mock.ExpectQuery(regexp.QuoteMeta(`SELECT 1 FROM user_status_message WHERE id=? AND user_id=?`)).WithArgs(int64(9), 7)
		rows := sqlmock.NewRows([]string{"exists"})
		if !missing {
			rows.AddRow(1)
		}
		expected.WillReturnRows(rows)
		if !missing {
			mock.ExpectExec(regexp.QuoteMeta(`UPDATE user_status_message SET read_at=COALESCE(read_at,CURRENT_TIMESTAMP) WHERE id=? AND user_id=?`)).WithArgs(int64(9), 7).WillReturnResult(sqlmock.NewResult(0, 0))
		}
		err = repo.MarkMessage(context.Background(), 7, 9, false, true)
		if missing && !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("foreign/absent message error=%v", err)
		}
		if !missing && err != nil {
			t.Fatal(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		raw.Close()
	}
}

func TestArchiveDisplayDoesNotReadOrChangeDeliveryQueue(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	repo := NewStatusNotificationRepository(sqlx.NewDb(raw, "sqlmock"))
	mock.ExpectQuery("SELECT 1 FROM user_status_message WHERE id=\\? AND user_id=\\?").WithArgs(int64(9), 7).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE user_status_message SET fullscreen_displayed=1,fullscreen_displayed_at=COALESCE(fullscreen_displayed_at,CURRENT_TIMESTAMP) WHERE id=? AND user_id=?`)).WithArgs(int64(9), 7).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.MarkMessage(context.Background(), 7, 9, true, false); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMessageListIsScopedFilteredOrderedAndDecodesSnapshots(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	repo := NewStatusNotificationRepository(sqlx.NewDb(raw, "sqlmock"))
	read := false
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM user_status_message WHERE user_id=? AND read_at IS NULL`)).WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(21))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT `+messageColumns+` FROM user_status_message WHERE user_id=? AND read_at IS NULL ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?`)).WithArgs(7, 20, 20).WillReturnRows(sqlmock.NewRows([]string{"id", "type", "payload", "read_at", "fullscreen_displayed", "fullscreen_displayed_at", "created_at"}).AddRow(9, "member-removed", `{"projectName":"archived project","projectId":42}`, nil, 0, nil, time.Now()))
	list, total, err := repo.ListMessages(context.Background(), 7, 2, 20, &read)
	if err != nil || total != 21 || len(list) != 1 {
		t.Fatalf("list=%v total=%d error=%v", list, total, err)
	}
	if list[0].Params["projectName"] != "archived project" || list[0].ReadAt != nil {
		t.Fatal("snapshot/read state lost")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPendingPrioritySuppressionDoesNotMarkArchiveRead(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	repo := NewStatusNotificationRepository(sqlx.NewDb(raw, "sqlmock"))
	mock.ExpectQuery(`(?s)SELECT sn.id.*WHERE sn.user_id = \? AND sn.displayed_at IS NULL\s+ORDER BY sn.priority DESC, sn.created_at DESC, sn.id DESC LIMIT 1`).WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "type", "priority", "message_id"}).AddRow(9, 7, "member-removed", 1000, 12))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE status_notification SET displayed_at=CURRENT_TIMESTAMP WHERE user_id=? AND displayed_at IS NULL AND id<>? AND type NOT IN ('cooperation-approved','cooperation-rejected')`)).WithArgs(7, int64(9)).WillReturnResult(sqlmock.NewResult(0, 3))
	item, err := repo.GetPending(context.Background(), 7)
	if err != nil || item.ID != 9 || *item.MessageID != 12 {
		t.Fatalf("item=%v error=%v", item, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRealDisplayCommitsQueueAndReadTogether(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	repo := NewStatusNotificationRepository(sqlx.NewDb(raw, "sqlmock"))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE status_notification SET displayed_at`).WithArgs(int64(9), 7).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE user_status_message SET read_at=.*fullscreen_displayed=1`).WithArgs(int64(9), 7).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := repo.MarkDisplayed(context.Background(), 9, 7); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveFailureRollsBackNotificationCreation(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	db := sqlx.NewDb(raw, "sqlmock")
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO status_notification`).WithArgs(7, "application-accepted", 42, 100).WillReturnResult(sqlmock.NewResult(9, 1))
	mock.ExpectExec(`INSERT INTO user_status_message`).WithArgs(int64(9)).WillReturnError(errors.New("archive unavailable"))
	mock.ExpectRollback()
	tx, err := db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := CreateTx(context.Background(), tx, 7, 42, "application-accepted"); err == nil {
		t.Fatal("expected archive failure")
	}
	tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
