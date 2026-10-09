package feishusync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type cleanupTransport func(*http.Request) (*http.Response, error)

func (f cleanupTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type defaultsFixture struct {
	blocks       []map[string]any
	schema       map[string]any
	views        map[string]any
	records      map[string]any
	dashboard    map[string]any
	workflow     map[string]any
	deleted      []string
	lists, auths int
	fail         string
	onList       func(int)
}

func newDefaultsFixture() *defaultsFixture {
	rows := make([][]any, 10)
	ids := make([]string, 10)
	for i := range rows {
		rows[i] = []any{nil, nil, nil, nil}
		ids[i] = fmt.Sprintf("rec-%d", i)
	}
	return &defaultsFixture{
		blocks: []map[string]any{
			{"id": "defaults-table", "name": "Table", "type": "table", "parent_id": nil, "records_count": 10, "rev": 0},
			{"id": "defaults-dashboard", "name": "Dashboard", "type": "dashboard", "parent_id": nil},
			{"id": "defaults-workflow", "name": "Workflow", "type": "workflow", "parent_id": nil},
			// A misleading renamed business table must never become a delete candidate.
			{"id": "business", "name": "Table", "type": "table", "parent_id": nil, "records_count": 0, "rev": 0},
		},
		schema: map[string]any{"total": 4, "fields": []map[string]any{
			{"id": "f1", "name": "Text", "type": "text", "default_value": nil, "style": map[string]any{"type": "plain"}},
			{"id": "f2", "name": "Single option", "type": "select", "multiple": false, "options": []any{}, "default_value": nil},
			{"id": "f3", "name": "Date", "type": "datetime", "style": map[string]any{"format": "yyyy/MM/dd"}, "default_value": nil},
			{"id": "f4", "name": "Attachment", "type": "attachment", "style": map[string]any{"type": "plain"}},
		}},
		views:     map[string]any{"total": 1, "views": []any{map[string]any{"id": "grid", "name": "Grid", "type": "grid", "_meta": map[string]any{"filter": nil, "group": []any{}, "sort": []any{}, "visible_fields": "4 fields"}}}},
		records:   map[string]any{"data": rows, "record_id_list": ids, "field_id_list": []string{"f1", "f2", "f3", "f4"}, "has_more": false, "rev": 0, "query_context": map[string]any{"record_scope": "all_records"}},
		dashboard: map[string]any{"dashboard_id": "defaults-dashboard", "name": "Dashboard", "blocks": []any{}, "theme": map[string]any{"theme_style": "default"}},
		workflow:  map[string]any{"workflow_id": "defaults-workflow", "title": "Workflow", "status": "disabled", "steps": []any{}, "create_time": 100, "update_time": 100, "creator_id": "bot", "updater_id": "bot"},
	}
}
func (f *defaultsFixture) client(t *testing.T) *Client {
	c := NewClient("test-app", "test-secret", "space", "parent")
	c.http = &http.Client{Transport: cleanupTransport(func(r *http.Request) (*http.Response, error) {
		// Isolated transport removes real waits; production retains shared pacing.
		c.requestMu.Lock()
		c.nextRequest = time.Time{}
		c.requestMu.Unlock()
		response := func(status int, value any) (*http.Response, error) {
			b, err := json.Marshal(value)
			require.NoError(t, err)
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(b))), Header: make(http.Header)}, nil
		}
		path := r.URL.Path
		if path == "/oauth/v3/token" {
			f.auths++
			require.Equal(t, "https://accounts.feishu.cn/oauth/v3/token", r.URL.String())
			require.NoError(t, r.ParseForm())
			require.Equal(t, "client_credentials", r.Form.Get("grant_type"))
			require.Equal(t, "test-app", r.Form.Get("client_id"))
			require.Equal(t, "test-secret", r.Form.Get("client_secret"))
			return response(200, map[string]any{"code": 0, "access_token": "base-token", "expires_in": 7200, "token_type": "Bearer"})
		}
		require.Equal(t, "Bearer base-token", r.Header.Get("Authorization"))
		require.True(t, strings.HasPrefix(path, "/open-apis/base/v3/bases/base/"))
		if f.fail != "" && strings.HasSuffix(path, f.fail) {
			return response(403, map[string]any{"code": 999, "msg": "private-remote-content"})
		}
		var data any
		switch {
		case strings.HasSuffix(path, "/blocks/list"):
			require.Equal(t, http.MethodPost, r.Method)
			f.lists++
			if f.onList != nil {
				f.onList(f.lists)
			}
			data = map[string]any{"blocks": f.blocks, "total": len(f.blocks)}
		case strings.HasSuffix(path, "/fields"):
			data = f.schema
		case strings.HasSuffix(path, "/views"):
			data = f.views
		case strings.HasSuffix(path, "/records"):
			require.Equal(t, []string{"f1", "f2", "f3", "f4"}, r.URL.Query()["field_id"])
			require.Equal(t, "11", r.URL.Query().Get("limit"))
			data = f.records
		case strings.HasSuffix(path, "/dashboards/defaults-dashboard"):
			data = f.dashboard
		case strings.HasSuffix(path, "/workflows/defaults-workflow"):
			data = f.workflow
		case r.Method == http.MethodDelete:
			id := path[strings.LastIndex(path, "/")+1:]
			require.NotEqual(t, "business", id)
			f.deleted = append(f.deleted, id)
			for i, b := range f.blocks {
				if b["id"] == id {
					f.blocks = append(f.blocks[:i], f.blocks[i+1:]...)
					break
				}
			}
			data = map[string]any{}
		default:
			t.Fatalf("unexpected cleanup request %s %s", r.Method, path)
		}
		return response(200, map[string]any{"code": 0, "data": data})
	})}
	return c
}
func cleanupTarget() Target { return Target{SchoolID: 10, AppToken: "base", TableID: "business"} }

func TestDefaultCleanupRemovesTenEmptyPlaceholdersAndIsRepeatable(t *testing.T) {
	f := newDefaultsFixture()
	// Normal business title allows the uniquely named default Table to be checked.
	f.blocks[3]["name"] = "用户名单"
	c := f.client(t)
	warning, err := c.CleanupDefaultBlocks(context.Background(), cleanupTarget(), func() error { return nil })
	require.NoError(t, err)
	require.False(t, warning)
	require.Equal(t, []string{"defaults-table", "defaults-dashboard", "defaults-workflow"}, f.deleted)
	require.Len(t, f.blocks, 1)
	before := f.lists
	warning, err = c.CleanupDefaultBlocks(context.Background(), cleanupTarget(), func() error { return nil })
	require.NoError(t, err)
	require.False(t, warning)
	require.Equal(t, before+1, f.lists)
	require.Equal(t, 1, f.auths)
	require.Len(t, f.deleted, 3)
}

func TestDefaultCleanupPreservesUsedModifiedAndIncompleteResources(t *testing.T) {
	cases := []struct {
		name      string
		change    func(*defaultsFixture)
		protected string
	}{
		{"nonempty cell", func(f *defaultsFixture) { f.records["data"].([][]any)[0][0] = "personal-content" }, "defaults-table"},
		{"modified revision", func(f *defaultsFixture) { f.blocks[0]["rev"] = 1 }, "defaults-table"},
		{"renamed", func(f *defaultsFixture) { f.blocks[0]["name"] = "My Table" }, "defaults-table"},
		{"custom field", func(f *defaultsFixture) { f.schema["fields"].([]map[string]any)[0]["name"] = "My column" }, "defaults-table"},
		{"custom view", func(f *defaultsFixture) { f.views["views"].([]any)[0].(map[string]any)["name"] = "My view" }, "defaults-table"},
		{"incomplete rows", func(f *defaultsFixture) { f.records["has_more"] = true }, "defaults-table"},
		{"missing completeness", func(f *defaultsFixture) { delete(f.records, "has_more") }, "defaults-table"},
		{"filtered rows", func(f *defaultsFixture) { f.records["query_context"] = map[string]any{"record_scope": "filtered"} }, "defaults-table"},
		{"unexpected column", func(f *defaultsFixture) { f.records["field_id_list"] = []string{"f1", "f2", "f3", "other"} }, "defaults-table"},
		{"missing rows", func(f *defaultsFixture) { f.records["data"] = [][]any{} }, "defaults-table"},
		{"configured dashboard", func(f *defaultsFixture) { f.dashboard["blocks"] = []any{map[string]any{"id": "chart"}} }, "defaults-dashboard"},
		{"missing dashboard contents", func(f *defaultsFixture) { delete(f.dashboard, "blocks") }, "defaults-dashboard"},
		{"enabled workflow", func(f *defaultsFixture) { f.workflow["status"] = "enabled" }, "defaults-workflow"},
		{"edited workflow", func(f *defaultsFixture) { f.workflow["update_time"] = 101 }, "defaults-workflow"},
		{"configured workflow", func(f *defaultsFixture) { f.workflow["steps"] = []any{map[string]any{"id": "step"}} }, "defaults-workflow"},
		{"ambiguous table", func(f *defaultsFixture) {
			f.blocks = append(f.blocks, map[string]any{"id": "manual", "name": "Table", "type": "table"})
		}, "defaults-table"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDefaultsFixture()
			f.blocks[3]["name"] = "用户名单"
			tc.change(f)
			warning, err := f.client(t).CleanupDefaultBlocks(context.Background(), cleanupTarget(), func() error { return nil })
			require.NoError(t, err)
			require.False(t, warning)
			require.NotContains(t, f.deleted, tc.protected)
		})
	}
}

func TestDefaultCleanupProtectsBusinessAndStopsAfterRevocation(t *testing.T) {
	f := newDefaultsFixture()
	_, err := f.client(t).CleanupDefaultBlocks(context.Background(), cleanupTarget(), func() error { return nil })
	require.NoError(t, err)
	require.NotContains(t, f.deleted, "business")
	require.NotContains(t, f.deleted, "defaults-table")
	f = newDefaultsFixture()
	f.blocks[3]["name"] = "用户名单"
	revoked := false
	f.onList = func(n int) {
		if n == 2 {
			revoked = true
		}
	}
	warning, err := f.client(t).CleanupDefaultBlocks(context.Background(), cleanupTarget(), func() error {
		if revoked {
			return &PermissionError{"revoked"}
		}
		return nil
	})
	require.Error(t, err)
	require.False(t, warning)
	require.Empty(t, f.deleted)
}

func TestDefaultCleanupRechecksDirectoryAndKeepsResourcesOnAPIFailure(t *testing.T) {
	for _, failure := range []string{"/blocks/list", "/records", "/blocks/defaults-table"} {
		t.Run(failure, func(t *testing.T) {
			f := newDefaultsFixture()
			f.blocks[3]["name"] = "用户名单"
			f.fail = failure
			warning, err := f.client(t).CleanupDefaultBlocks(context.Background(), cleanupTarget(), func() error { return nil })
			require.NoError(t, err)
			require.True(t, warning)
			require.NotContains(t, f.deleted, "defaults-table")
		})
	}
	f := newDefaultsFixture()
	f.blocks[3]["name"] = "用户名单"
	f.onList = func(n int) {
		if n == 2 {
			f.blocks[0]["rev"] = 1
		}
	}
	warning, err := f.client(t).CleanupDefaultBlocks(context.Background(), cleanupTarget(), func() error { return nil })
	require.NoError(t, err)
	require.False(t, warning)
	require.NotContains(t, f.deleted, "defaults-table")
	f = newDefaultsFixture()
	f.blocks = f.blocks[:3]
	warning, err = f.client(t).CleanupDefaultBlocks(context.Background(), cleanupTarget(), func() error { return nil })
	require.NoError(t, err)
	require.True(t, warning)
	require.Empty(t, f.deleted)
}

type fakeDefaultCleaner struct {
	fakeRemote
	warning bool
	err     error
	calls   int
}

func (f *fakeDefaultCleaner) CleanupDefaultBlocks(_ context.Context, _ Target, guard func() error) (bool, error) {
	f.calls++
	if err := guard(); err != nil {
		return false, err
	}
	return f.warning, f.err
}
func TestSchoolSyncRunsCleanupAfterDataAndRetainsWarning(t *testing.T) {
	for _, warning := range []bool{false, true} {
		t.Run(fmt.Sprint(warning), func(t *testing.T) {
			store, repo, mock := testStore(t)
			remote := &fakeDefaultCleaner{warning: warning}
			s := &Service{store: store, repo: repo, remote: remote}
			mock.ExpectQuery("SELECT school_id,app_token.*FROM feishu_user_sync_target").WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"school_id", "app_token", "node_token", "table_id", "view_id", "node_started", "table_started"}).AddRow(10, "base", "node", "business", "view", true, true))
			mock.ExpectQuery("SELECT r.user_id.*WHERE r.school_id=.*AND u.id IS NULL").WithArgs(10).WillReturnRows(recordRows())
			mock.ExpectQuery("SELECT u.id FROM.*ORDER BY u.id").WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"id"}))
			expectProgress(mock)
			mock.ExpectQuery("SELECT r.user_id.*WHERE r.school_id=.*AND u.id IS NULL").WithArgs(10).WillReturnRows(recordRows())
			job := testJob(0)
			require.NoError(t, s.syncSchool(context.Background(), job, func() error { return nil }))
			require.Equal(t, 1, remote.calls)
			if warning {
				require.Equal(t, defaultCleanupWarning, job.Message)
			} else {
				require.Equal(t, "同步完成", job.Message)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestDefaultCleanupAuthRejectionDoesNotExposeCredentials(t *testing.T) {
	c := NewClient("app", "secret", "space", "parent")
	c.http = &http.Client{Transport: cleanupTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("secret transport error") })}
	_, err := c.baseAccessToken(context.Background())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret")
	warning, err := c.CleanupDefaultBlocks(context.Background(), cleanupTarget(), func() error { return nil })
	require.NoError(t, err)
	require.True(t, warning)
}

func TestDefaultCleanupStopsIfDuplicateAppearsDuringCheck(t *testing.T) {
	f := newDefaultsFixture()
	f.blocks[3]["name"] = "用户名单"
	f.onList = func(n int) {
		if n == 2 {
			f.blocks = append(f.blocks, map[string]any{"id": "other", "name": "Table", "type": "table"})
		}
	}
	warning, err := f.client(t).CleanupDefaultBlocks(context.Background(), cleanupTarget(), func() error { return nil })
	require.NoError(t, err)
	require.False(t, warning)
	require.NotContains(t, f.deleted, "defaults-table")
}

func TestNewSchoolAlsoRunsDefaultCleanupOnlyAfterProvisioning(t *testing.T) {
	store, repo, mock := testStore(t)
	remote := &fakeDefaultCleaner{}
	s := &Service{store: store, repo: repo, remote: remote}
	mock.ExpectQuery("SELECT school_id,app_token.*FROM feishu_user_sync_target").WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"school_id", "app_token", "node_token", "table_id", "view_id", "node_started", "table_started"}).AddRow(10, "", "", "", "", false, false))
	mock.ExpectQuery("SELECT id, school_name.*FROM school").WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"id", "school_name"}).AddRow(10, "学校10"))
	for i := 0; i < 4; i++ {
		mock.ExpectExec("UPDATE feishu_user_sync_target SET app_token=").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectQuery("SELECT r.user_id.*WHERE r.school_id=.*AND u.id IS NULL").WithArgs(10).WillReturnRows(recordRows())
	mock.ExpectQuery("SELECT u.id FROM.*ORDER BY u.id").WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	expectProgress(mock)
	mock.ExpectQuery("SELECT r.user_id.*WHERE r.school_id=.*AND u.id IS NULL").WithArgs(10).WillReturnRows(recordRows())
	require.NoError(t, s.syncSchool(context.Background(), testJob(0), func() error { return nil }))
	require.Equal(t, 1, remote.calls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBaseCleanupAuthValidationAndTokenSeparation(t *testing.T) {
	for _, body := range []string{`{"code":0,"access_token":"token","expires_in":7200,"token_type":"DPoP"}`, `{"code":0,"access_token":"token","expires_in":1,"token_type":"Bearer"}`, `{"access_token":"token","expires_in":7200,"token_type":"Bearer"}`, `{"code":999,"msg":"secret content"}`, `{`} {
		c := NewClient("app", "secret", "space", "parent")
		c.token = "existing-v1-token"
		c.expires = time.Now().Add(time.Hour)
		c.http = &http.Client{Transport: cleanupTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})}
		_, err := c.baseAccessToken(context.Background())
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
		require.Empty(t, c.baseToken)
		token, err := c.accessToken(context.Background())
		require.NoError(t, err)
		require.Equal(t, "existing-v1-token", token)
	}
}
