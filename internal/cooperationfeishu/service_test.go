package cooperationfeishu

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"

	"github.com/stretchr/testify/require"
)

func TestApplicationValuesOnlyUseCapturedCardFields(t *testing.T) {
	card := `{"nickname":"同学","phone":"13800000000","skills":["Go","设计"],"unrelatedSecret":"must-not-export"}`
	values, err := (application{ID: 5, UserID: 7, AttemptNo: 2, Status: "pending", Profile: &card}).values()
	require.NoError(t, err)
	require.Equal(t, "13800000000", values["电话"])
	require.Equal(t, "未处理", values["申请结果"])
	require.NotContains(t, values, "unrelatedSecret")
}

func TestApplicationClientTokensRequireUUIDv4(t *testing.T) {
	require.True(t, isUUIDv4("550e8400-e29b-41d4-a716-446655440000"))
	require.False(t, isUUIDv4("cooperation-application-1"))
	require.False(t, isUUIDv4("550e8400-e29b-11d4-a716-446655440000"))
}

// Exercise the real response decoding and matching, without writing to Feishu.
type remoteFunc func(context.Context, string, string, any, any) error

func (f remoteFunc) Call(ctx context.Context, method, path string, body, out any) error {
	return f(ctx, method, path, body, out)
}
func fieldMaps(t *testing.T) []map[string]any {
	t.Helper()
	encoded, err := json.Marshal(fields())
	require.NoError(t, err)
	var result []map[string]any
	require.NoError(t, json.Unmarshal(encoded, &result))
	return result
}
func TestTableSchemaMatchesRequiredFieldsIndependentlyOfOrderAndExtraColumns(t *testing.T) {
	items := fieldMaps(t)
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	items = append(items, map[string]any{"field_name": "学校人工备注", "type": 1})
	calls := 0
	s := &Service{client: remoteFunc(func(_ context.Context, method, path string, _ any, out any) error {
		calls++
		require.Equal(t, http.MethodGet, method)
		require.Contains(t, path, "/fields?page_size=100")
		raw, err := json.Marshal(map[string]any{"items": items, "has_more": false})
		require.NoError(t, err)
		return json.Unmarshal(raw, out)
	})}
	mismatch, err := s.tableSchemaMismatch(context.Background(), "app", "table")
	require.NoError(t, err)
	require.False(t, mismatch)
	require.Equal(t, 1, calls)
}
func TestTableSchemaReadsAllPagesBeforeCheckingRequiredFields(t *testing.T) {
	calls := 0
	s := &Service{client: remoteFunc(func(_ context.Context, _ string, path string, _ any, out any) error {
		calls++
		var response map[string]any
		if calls == 1 {
			response = map[string]any{"items": []map[string]any{{"field_name": "人工备注", "type": 1}}, "has_more": true, "page_token": "next/page"}
		} else {
			require.Equal(t, 2, calls)
			require.True(t, strings.HasSuffix(path, "&page_token=next%2Fpage"))
			response = map[string]any{"items": fieldMaps(t), "has_more": false}
		}
		raw, err := json.Marshal(response)
		require.NoError(t, err)
		return json.Unmarshal(raw, out)
	})}
	mismatch, err := s.tableSchemaMismatch(context.Background(), "app", "table")
	require.NoError(t, err)
	require.False(t, mismatch)
	require.Equal(t, 2, calls)
}
func TestTableSchemaStillRejectsIncompatibleRequiredFields(t *testing.T) {
	for _, kind := range []string{"missing", "type", "option"} {
		t.Run(kind, func(t *testing.T) {
			items := fieldMaps(t)
			switch kind {
			case "missing":
				items = items[1:]
			case "type":
				items[0]["type"] = 99
			case "option":
				for _, item := range items {
					if item["field_name"] == "申请结果" {
						item["property"] = map[string]any{"options": []any{}}
					}
				}
			}
			s := &Service{client: remoteFunc(func(_ context.Context, _ string, _ string, _ any, out any) error {
				raw, err := json.Marshal(map[string]any{"items": items})
				require.NoError(t, err)
				return json.Unmarshal(raw, out)
			})}
			mismatch, err := s.tableSchemaMismatch(context.Background(), "app", "table")
			require.NoError(t, err)
			require.True(t, mismatch)
		})
	}
}
func TestIncompleteSchemaReadCannotTriggerReplacement(t *testing.T) {
	for _, kind := range []string{"api error", "missing token", "repeated token"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			s := &Service{client: remoteFunc(func(_ context.Context, _ string, _ string, _ any, out any) error {
				calls++
				if kind == "api error" {
					return errors.New("unavailable")
				}
				token := ""
				if kind == "repeated token" {
					token = "same"
				}
				raw, err := json.Marshal(map[string]any{"items": []any{}, "has_more": true, "page_token": token})
				require.NoError(t, err)
				return json.Unmarshal(raw, out)
			})}
			mismatch, err := s.tableSchemaMismatch(context.Background(), "app", "table")
			require.Error(t, err)
			require.False(t, mismatch)
			require.LessOrEqual(t, calls, 2)
		})
	}
}
func TestMarkRunningLocksBindingAndCommitsBeforeLaunch(t *testing.T) {
	raw, m, err := sqlmock.New()
	require.NoError(t, err)
	defer raw.Close()
	s := &Service{db: sqlx.NewDb(raw, "sqlmock")}
	m.ExpectBegin()
	m.ExpectQuery(regexp.QuoteMeta(`SELECT feishu_wiki_node_token FROM commercial_order WHERE id=? FOR UPDATE`)).WithArgs(int64(3)).WillReturnRows(sqlmock.NewRows([]string{"feishu_wiki_node_token"}).AddRow("boundnode"))
	m.ExpectExec(regexp.QuoteMeta(`UPDATE cooperation_feishu_target SET status='running',message='正在同步' WHERE cooperation_id=? AND status IN ('idle','succeeded','failed')`)).WithArgs(int64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	changed, err := s.markRunning(context.Background(), 3, 9, "'idle','succeeded','failed'")
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, m.ExpectationsWereMet())
}
func TestMarkRunningRechecksBindingAndNeverLaunchesOnFailure(t *testing.T) {
	for _, kind := range []string{"unbound", "update error", "commit error", "already running"} {
		t.Run(kind, func(t *testing.T) {
			raw, m, err := sqlmock.New()
			require.NoError(t, err)
			defer raw.Close()
			s := &Service{db: sqlx.NewDb(raw, "sqlmock")}
			m.ExpectBegin()
			rows := sqlmock.NewRows([]string{"feishu_wiki_node_token"})
			if kind == "unbound" {
				rows.AddRow(nil)
			} else {
				rows.AddRow("boundnode")
			}
			m.ExpectQuery(`SELECT feishu_wiki_node_token FROM commercial_order WHERE id=\? FOR UPDATE`).WithArgs(int64(3)).WillReturnRows(rows)
			if kind == "unbound" {
				m.ExpectRollback()
			} else {
				exec := m.ExpectExec(`UPDATE cooperation_feishu_target SET`).WithArgs(int64(9))
				if kind == "update error" {
					exec.WillReturnError(errors.New("write failed"))
					m.ExpectRollback()
				} else {
					changed := int64(1)
					if kind == "already running" {
						changed = 0
					}
					exec.WillReturnResult(sqlmock.NewResult(0, changed))
					commit := m.ExpectCommit()
					if kind == "commit error" {
						commit.WillReturnError(errors.New("commit failed"))
					}
				}
			}
			changed, err := s.markRunning(context.Background(), 3, 9, "'idle','succeeded','failed'")
			if kind == "already running" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.False(t, changed)
			require.NoError(t, m.ExpectationsWereMet())
		})
	}
}
