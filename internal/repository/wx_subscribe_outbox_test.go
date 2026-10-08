package repository

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"testing"
)

func TestSubscribeIntentRollsBackWithBusinessTransaction(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	db := sqlx.NewDb(raw, "mysql")
	mock.ExpectBegin()
	tx, err := db.BeginTxx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("INSERT INTO wx_subscribe_delivery").WithArgs(7, "KEY", "{}", nil, "PENDING").WillReturnError(errors.New("outbox down"))
	mock.ExpectRollback()
	if _, err := CreateWxSubscribeDeliveryTx(context.Background(), tx, &models.WxSubscribeDelivery{UserID: 7, BizKey: "KEY", BusinessData: "{}", Status: "PENDING"}); err == nil {
		t.Fatal("partial commit must be prevented")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
