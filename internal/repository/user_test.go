package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
)

var (
	captureDriverOnce sync.Once
	capturedExec      struct {
		sync.Mutex
		query string
		args  []driver.NamedValue
	}
)

func TestTouchLastActiveDateResetsDailyFreeBranchQuota(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()

	capturedExec.Lock()
	capturedExec.query = ""
	capturedExec.args = nil
	capturedExec.Unlock()

	repo := NewUserRepository(sqlx.NewDb(db, "capture_user_repo"))
	if err := repo.TouchLastActiveDate(context.Background(), 42); err != nil {
		t.Fatalf("TouchLastActiveDate returned error: %v", err)
	}

	capturedExec.Lock()
	query := normalizeSQL(capturedExec.query)
	args := append([]driver.NamedValue(nil), capturedExec.args...)
	capturedExec.Unlock()

	for _, want := range []string{
		"SET free_branch_used_today = 0, last_active_date = CURDATE()",
		"WHERE id = ? AND (last_active_date IS NULL OR last_active_date < CURDATE())",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query = %q, want to contain %q", query, want)
		}
	}
	if len(args) != 1 || args[0].Value != int64(42) {
		t.Fatalf("args = %#v, want single user id 42", args)
	}
}

func openCaptureDB(t *testing.T) *sql.DB {
	t.Helper()
	captureDriverOnce.Do(func() {
		sql.Register("capture_user_repo", captureDriver{})
	})
	db, err := sql.Open("capture_user_repo", "")
	if err != nil {
		t.Fatalf("open capture db: %v", err)
	}
	return db
}

func normalizeSQL(query string) string {
	return strings.Join(strings.Fields(query), " ")
}

type captureDriver struct{}

func (captureDriver) Open(string) (driver.Conn, error) {
	return captureConn{}, nil
}

type captureConn struct{}

func (captureConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not implemented")
}

func (captureConn) Close() error {
	return nil
}

func (captureConn) Begin() (driver.Tx, error) {
	return captureTx{}, nil
}

func (captureConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	capturedExec.Lock()
	capturedExec.query = query
	capturedExec.args = append([]driver.NamedValue(nil), args...)
	capturedExec.Unlock()
	return driver.RowsAffected(1), nil
}

type captureTx struct{}

func (captureTx) Commit() error {
	return nil
}

func (captureTx) Rollback() error {
	return nil
}
