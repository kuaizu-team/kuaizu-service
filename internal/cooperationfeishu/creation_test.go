package cooperationfeishu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/feishusync"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCreationErrorKeepsUncertainOutcomesBlocked(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		safe bool
	}{
		{"wiki permission", &feishusync.APIError{Status: 400, Code: 131006}, true},
		{"base permission", &feishusync.APIError{Status: 403, Code: 1254302}, true},
		{"scope in HTTP 200", &feishusync.APIError{Status: 200, Code: 99991672}, true},
		{"unauthorized", &feishusync.APIError{Status: 401, Code: 99991663}, true},
		{"throttled", &feishusync.APIError{Status: 429, Code: 99991400}, true},
		{"internal error in HTTP 400", &feishusync.APIError{Status: 400, Code: 1255001}, false},
		{"unknown business error", &feishusync.APIError{Status: 200, Code: 123456}, false},
		{"server error", &feishusync.APIError{Status: 500, Code: 131006}, false},
		{"request timeout", &feishusync.APIError{Status: 408, Code: 131006}, false},
		{"unexpected redirect", &feishusync.APIError{Status: 302, Code: 131006}, false},
		{"zero code", &feishusync.APIError{Status: 403}, false},
		{"network or malformed response", errors.New("unconfirmed response"), false},
		{"cancelled", context.Canceled, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Wrapping must not discard the original sanitized API error used by logs.
			err := creationError(fmt.Errorf("create: %w", tc.err))
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, !tc.safe, errors.Is(err, errNeedsAttention))
		})
	}
}

func expectSyncDetail(m sqlmock.Sqlmock) {
	m.ExpectQuery(regexp.QuoteMeta(`SELECT role,school_id,status FROM admin_user WHERE id=?`)).WithArgs(7).
		WillReturnRows(sqlmock.NewRows([]string{"role", "school_id", "status"}).AddRow(1, nil, 1))
	m.ExpectQuery(`SELECT c.id.*WHERE c.id=\?$`).WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "commercial_order_id", "school_id", "company_name", "school_name", "feishu_wiki_node_token"}).AddRow(9, 3, 42, "公司", "学校", "bound-node"))
}

// Exercise real run -> sync -> creation -> persisted status, at both creation
// stages. No goroutine or Feishu write is needed to verify the recovery gate.
func TestRunPersistsRetryableCreationFailuresAndProtectsUnknownResults(t *testing.T) {
	for _, stage := range []string{"wiki", "table"} {
		for _, outcome := range []string{"permission", "timeout", "missing ID", "mapping write failed"} {
			t.Run(stage+"/"+outcome, func(t *testing.T) {
				raw, m, err := sqlmock.New()
				require.NoError(t, err)
				defer raw.Close()
				db := sqlx.NewDb(raw, "sqlmock")
				s := &Service{db: db, cooperation: service.NewCampusCooperationService(repository.New(db), nil, nil), spaceID: "space"}
				expectSyncDetail(m)
				rows := sqlmock.NewRows([]string{"cooperation_id", "app_token", "table_id", "doc_block_id", "status"})
				if stage == "wiki" {
					rows.AddRow(9, nil, nil, nil, "running")
				} else {
					rows.AddRow(9, "base", nil, "wiki:school-node", "running")
				}
				m.ExpectQuery(regexp.QuoteMeta(`SELECT ` + targetColumns + ` FROM cooperation_feishu_target WHERE cooperation_id=?`)).WithArgs(int64(9)).WillReturnRows(rows)
				calls := 0
				s.client = remoteFunc(func(_ context.Context, method, path string, _ any, out any) error {
					calls++
					if calls == 1 {
						require.Equal(t, http.MethodGet, method)
						return json.Unmarshal([]byte(`{"node":{"space_id":"space","obj_type":"docx","obj_token":"document"}}`), out)
					}
					// Table-stage retries reuse the mapped Base; they must not create a new Wiki node.
					require.Equal(t, http.MethodPost, method)
					if stage == "wiki" {
						require.Equal(t, "/open-apis/wiki/v2/spaces/space/nodes", path)
					} else {
						require.Equal(t, "/open-apis/bitable/v1/apps/base/tables", path)
					}
					switch outcome {
					case "permission":
						return &feishusync.APIError{Status: 403, Code: 1254302}
					case "timeout":
						return context.DeadlineExceeded
					case "missing ID":
						return json.Unmarshal([]byte(`{}`), out)
					default:
						return json.Unmarshal([]byte(`{"node":{"node_token":"school-node","obj_token":"base"},"table_id":"table","default_view_id":"view"}`), out)
					}
				})
				if outcome == "mapping write failed" {
					if stage == "wiki" {
						m.ExpectBegin()
						m.ExpectExec(`DELETE FROM cooperation_feishu_record`).WithArgs(int64(9)).WillReturnError(errors.New("DB unavailable"))
						m.ExpectRollback()
					} else {
						m.ExpectExec(`UPDATE cooperation_feishu_target SET table_id=`).WithArgs("table", "view", int64(9)).WillReturnError(errors.New("DB unavailable"))
					}
				}
				status, message := "needs_attention", "远端创建结果待核对，请联系技术人员确认后重试"
				if outcome == "permission" {
					status, message = "failed", "同步失败，请检查应用授权和表格权限"
				}
				m.ExpectExec(`UPDATE cooperation_feishu_target SET status=\?,message=\?,synced_at=`).
					WithArgs(status, message, status, int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
				syncSlot <- struct{}{}
				s.run(9, 7)
				require.Equal(t, 2, calls)
				require.Empty(t, syncSlot)
				require.NoError(t, m.ExpectationsWereMet())
			})
		}
	}
}

func TestStartRelaunchesFailedCreationWithoutRemoteMapping(t *testing.T) {
	raw, m, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	// Start's response read and the background worker may read in either order.
	m.MatchExpectationsInOrder(false)
	db := sqlx.NewDb(raw, "sqlmock")
	s := &Service{db: db, cooperation: service.NewCampusCooperationService(repository.New(db), nil, nil), spaceID: "space"}
	for i := 0; i < 3; i++ {
		expectSyncDetail(m)
		status := "running"
		if i == 0 {
			status = "failed"
		}
		m.ExpectQuery(regexp.QuoteMeta(`SELECT ` + targetColumns + ` FROM cooperation_feishu_target WHERE cooperation_id=?`)).WithArgs(int64(9)).
			WillReturnRows(sqlmock.NewRows([]string{"cooperation_id", "status", "updated_at"}).AddRow(9, status, time.Now()))
	}
	m.ExpectExec(`INSERT IGNORE INTO cooperation_feishu_target`).WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectBegin()
	m.ExpectQuery(`SELECT feishu_wiki_node_token FROM commercial_order WHERE id=\? FOR UPDATE`).WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"feishu_wiki_node_token"}).AddRow("bound-node"))
	m.ExpectExec(regexp.QuoteMeta(`UPDATE cooperation_feishu_target SET status='running',message='正在同步' WHERE cooperation_id=? AND status IN ('idle','succeeded','failed')`)).
		WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	// A subsequent permission rejection remains retryable instead of poisoning the gate.
	s.client = remoteFunc(func(_ context.Context, method, path string, _ any, out any) error {
		if method == http.MethodGet {
			return json.Unmarshal([]byte(`{"node":{"space_id":"space","obj_type":"docx","obj_token":"document"}}`), out)
		}
		return &feishusync.APIError{Status: 400, Code: 131006}
	})
	m.ExpectExec(`UPDATE cooperation_feishu_target SET status=\?,message=\?,synced_at=`).
		WithArgs("failed", "同步失败，请检查应用授权和表格权限", "failed", int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	target, startErr := s.Start(context.Background(), 7, 9)
	require.Eventually(t, func() bool { return m.ExpectationsWereMet() == nil && len(syncSlot) == 0 }, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, startErr)
	require.Equal(t, int64(9), target.CooperationID)
}
