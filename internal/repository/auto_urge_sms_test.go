package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestAutoUrgeCycleClaims(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name, state                           string
		key                                   any
		closed                                any
		due, stale, want, reconcile, newCycle bool
		cycle                                 int64
	}{
		{"initial", "ready", nil, nil, true, false, true, false, false, 1},
		{"same backlog sent", "sent", "auto-urge:7:1", nil, true, false, false, false, false, 1},
		{"cleared successful cycle", "sent", "auto-urge:7:1", now, true, false, true, false, true, 2},
		{"definite failure same key", "failed", "auto-urge:7:1", nil, true, false, true, false, false, 1},
		{"failed not due", "failed", "auto-urge:7:1", nil, false, false, false, false, false, 1},
		{"timeout lookup", "unknown", "auto-urge:7:1", nil, true, false, true, true, false, 1},
		{"cleared timeout still old key", "unknown", "auto-urge:7:1", now, true, false, true, true, false, 1},
		{"stale lease lookup", "claimed", "auto-urge:7:1", nil, true, true, true, true, false, 1},
		{"live lease blocked", "claimed", "auto-urge:7:1", nil, true, false, false, false, false, 1},
		{"legacy timeout blocked", "unknown", nil, now, true, false, false, false, false, 1},
		{"legacy claimed blocked", "claimed", nil, now, true, true, false, false, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, m, err := sqlmock.New()
			require.NoError(t, err)
			defer raw.Close()
			r := NewAutoUrgeRepository(sqlx.NewDb(raw, "mysql"))
			m.ExpectBegin()
			m.ExpectExec("INSERT INTO auto_urge_sms_state").WithArgs(7).WillReturnResult(sqlmock.NewResult(0, 0))
			m.ExpectQuery("(?s)SELECT state,cycle_id.*FOR UPDATE").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"state", "cycle_id", "request_key", "closed_at", "due", "stale"}).AddRow(tc.state, 1, tc.key, tc.closed, tc.due, tc.stale))
			key := "auto-urge:7:1"
			if tc.newCycle {
				key = "auto-urge:7:2"
			}
			if tc.want {
				m.ExpectExec("(?s)UPDATE auto_urge_sms_state SET cycle_id=").WithArgs(tc.cycle, key, "token", tc.newCycle, tc.newCycle, tc.newCycle, tc.newCycle, tc.newCycle, 7).WillReturnResult(sqlmock.NewResult(0, 1))
				m.ExpectCommit()
			} else {
				m.ExpectRollback()
			}
			claim, err := r.Claim(context.Background(), 7, "token")
			require.NoError(t, err)
			if tc.want {
				require.NotNil(t, claim)
				require.Equal(t, tc.cycle, claim.CycleID)
				require.Equal(t, key, claim.RequestKey)
				require.Equal(t, tc.reconcile, claim.ReconcileOnly)
			} else {
				require.Nil(t, claim)
			}
			require.NoError(t, m.ExpectationsWereMet())
		})
	}
}
func TestAutoUrgeClaimFailureRollsBack(t *testing.T) {
	raw, m, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	r := NewAutoUrgeRepository(sqlx.NewDb(raw, "mysql"))
	m.ExpectBegin()
	m.ExpectExec("INSERT INTO auto_urge_sms_state").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectQuery("(?s)SELECT state,cycle_id.*FOR UPDATE").WillReturnError(errors.New("database unavailable"))
	m.ExpectRollback()
	claim, err := r.Claim(context.Background(), 7, "token")
	require.Error(t, err)
	require.Nil(t, claim)
	require.NoError(t, m.ExpectationsWereMet())
}
func TestAutoUrgeLeaseFences(t *testing.T) {
	claim := AutoUrgeClaim{UserID: 7, CycleID: 2, Token: "new-owner", RequestKey: "auto-urge:7:2"}
	for _, operation := range []string{"snapshot", "dispatch", "finish"} {
		t.Run(operation, func(t *testing.T) {
			raw, m, err := sqlmock.New()
			require.NoError(t, err)
			defer raw.Close()
			r := NewAutoUrgeRepository(sqlx.NewDb(raw, "mysql"))
			switch operation {
			case "snapshot":
				m.ExpectExec("(?s)UPDATE auto_urge_sms_state SET pending_count.*claim_token=\\? AND cycle_id=\\? AND closed_at IS NULL").
					WithArgs(3, nil, 7, "new-owner", 2).WillReturnResult(sqlmock.NewResult(0, 0))
				m.ExpectQuery("(?s)SELECT EXISTS.*cycle_id=\\? AND closed_at IS NULL").WithArgs(7, "new-owner", 2).
					WillReturnRows(sqlmock.NewRows([]string{"owns"}).AddRow(false))
				err = r.Snapshot(context.Background(), AutoUrgeCandidate{UserID: 7, PendingCount: 3}, claim)
			case "dispatch":
				m.ExpectExec("(?s)UPDATE auto_urge_sms_state SET dispatched_at.*claim_token=\\? AND cycle_id=\\? AND request_key=\\? AND closed_at IS NULL").
					WithArgs(7, "new-owner", 2, "auto-urge:7:2").WillReturnResult(sqlmock.NewResult(0, 0))
				err = r.MarkDispatched(context.Background(), claim)
			case "finish":
				m.ExpectExec("(?s)UPDATE auto_urge_sms_state SET state=.*claim_token=\\? AND cycle_id=\\? AND request_key=\\?").
					WithArgs("sent", nil, "", "", "sent", "sent", 7, "new-owner", 2, "auto-urge:7:2").WillReturnResult(sqlmock.NewResult(0, 0))
				err = r.Finish(context.Background(), claim, AutoUrgeResult{State: "sent"})
			}
			require.ErrorIs(t, err, ErrAutoUrgeClaimLost)
			require.NoError(t, m.ExpectationsWereMet())
		})
	}
}
func TestAutoUrgeSnapshotUnchangedOwned(t *testing.T) {
	raw, m, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	r := NewAutoUrgeRepository(sqlx.NewDb(raw, "mysql"))
	claim := AutoUrgeClaim{UserID: 7, CycleID: 1, Token: "token"}
	m.ExpectExec("UPDATE auto_urge_sms_state SET pending_count").WithArgs(3, nil, 7, "token", 1).WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectQuery("SELECT EXISTS").WithArgs(7, "token", 1).WillReturnRows(sqlmock.NewRows([]string{"owns"}).AddRow(true))
	require.NoError(t, r.Snapshot(context.Background(), AutoUrgeCandidate{UserID: 7, PendingCount: 3}, claim))
	require.NoError(t, m.ExpectationsWereMet())
}
func TestAutoUrgeClearanceScopeAndTransaction(t *testing.T) {
	raw, m, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	r := NewAutoUrgeRepository(sqlx.NewDb(raw, "mysql"))
	// Disabled tracking makes no new query, including when old schema is deployed.
	require.NoError(t, r.ObserveClearanceTx(context.Background(), nil, 7))
	r.EnableTracking()
	m.ExpectBegin()
	tx, err := r.db.BeginTxx(context.Background(), nil)
	require.NoError(t, err)
	m.ExpectExec("(?s)UPDATE auto_urge_sms_state a SET closed_at=NOW\\(\\).*user_id IN \\(\\?, \\?\\).*NOT EXISTS.*ob.status=0.*deleted_at IS NULL.*NOT EXISTS.*pa.status=0.*project_members").
		WithArgs(7, 9).WillReturnError(errors.New("write failed"))
	m.ExpectRollback()
	err = r.ObserveClearanceTx(context.Background(), tx, 9, 7, 7, 0)
	require.Error(t, err)
	require.NoError(t, tx.Rollback())
	require.NoError(t, m.ExpectationsWereMet())
}
func TestAutoUrgeCandidateKeyset(t *testing.T) {
	raw, m, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	r := NewAutoUrgeRepository(sqlx.NewDb(raw, "mysql"))
	m.ExpectQuery("(?s)WITH reviewers.*COUNT\\(\\*\\)>=3 OR MIN\\(pending_at\\).*168 HOUR.*a.state='sent' AND a.closed_at IS NOT NULL.*a.state='unknown'.*30 MINUTE.*ORDER BY u.id LIMIT").
		WithArgs(5, 200).WillReturnRows(sqlmock.NewRows([]string{"user_id", "nickname", "pending_count", "oldest_pending_at"}).AddRow(7, "用户", 3, time.Now()).AddRow(9, "已清空待核验", 0, nil))
	rows, err := r.Candidates(context.Background(), 5, 200)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.NoError(t, m.ExpectationsWereMet())
}
func TestAutoUrgeRecheckNoEligibleWork(t *testing.T) {
	raw, m, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	r := NewAutoUrgeRepository(sqlx.NewDb(raw, "mysql"))
	m.ExpectQuery("(?s)WITH pending.*JOIN project.*ob.status=0.*deleted_at IS NULL").WithArgs(7, 7, 7, 7).
		WillReturnError(sql.ErrNoRows)
	c, err := r.Recheck(context.Background(), 7)
	require.NoError(t, err)
	require.Nil(t, c)
	require.NoError(t, m.ExpectationsWereMet())
}
