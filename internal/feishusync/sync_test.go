package feishusync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) (*Store, *repository.Repository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	x := sqlx.NewDb(db, "mysql")
	return NewStore(x), repository.New(x), mock
}

func TestLegacy15ColumnContractAndRawValues(t *testing.T) {
	var names []string
	for _, field := range Fields() {
		names = append(names, field.Name)
	}
	require.Equal(t, []string{"昵称", "MBTI", "学校", "专业", "入学年份", "自我介绍", "项目经历", "协作等级", "协作具体分数值", "是否通过学生认证", "是否入驻人才库", "电话", "微信号", "邮箱号", "账号状态"}, names)
	row := UserRow{ID: 7, Nickname: "=HYPERLINK(1)", MBTI: "INTJ", Grade: "2", Intro: "第一行\n第二行", Phone: "00123", Score: sql.NullFloat64{Float64: 98.5, Valid: true}, Auth: 1, Talent: 1, Status: 2}
	values, err := row.Values()
	require.NoError(t, err)
	require.Len(t, values, 15)
	require.Equal(t, "=HYPERLINK(1)", values["昵称"])
	require.Equal(t, "第一行\n第二行", values["自我介绍"])
	require.Equal(t, "2", values["入学年份"])
	require.Equal(t, "00123", values["电话"])
	require.Equal(t, "极好", values["协作等级"])
	require.Equal(t, 98.5, values["协作具体分数值"])
	require.Equal(t, "是", values["是否通过学生认证"])
	require.Equal(t, "已入驻人才库", values["是否入驻人才库"])
	require.Equal(t, "已毕业", values["账号状态"])
	require.NotContains(t, values, "用户ID")
	row.Score.Valid = false
	row.MBTI = ""
	row.Auth = 3
	row.Talent = 2
	row.Status = 1
	values, err = row.Values()
	require.NoError(t, err)
	require.Nil(t, values["协作等级"])
	require.Nil(t, values["协作具体分数值"])
	require.Nil(t, values["MBTI"])
	require.Equal(t, "否", values["是否通过学生认证"])
	require.Equal(t, "未入驻人才库", values["是否入驻人才库"])
	require.Equal(t, "封禁", values["账号状态"])
	row.Score = sql.NullFloat64{Float64: 59.995, Valid: true}
	values, err = row.Values()
	require.NoError(t, err)
	require.Equal(t, 59.99, values["协作具体分数值"])
	row.MBTI = "unexpected"
	_, err = row.Values()
	require.Error(t, err)
}

func TestSchemaDriftStopsBeforeWriting(t *testing.T) {
	require.NoError(t, ValidateFields(Fields()))
	fields := Fields()
	fields[1].Type = 1
	require.Error(t, ValidateFields(fields))
	fields = Fields()
	fields[1], fields[2] = fields[2], fields[1]
	require.Error(t, ValidateFields(fields))
	fields = Fields()
	fields[9].Property.Options = nil
	require.Error(t, ValidateFields(fields))
	require.Error(t, ValidateFields(append(Fields(), Field{Name: "用户ID", Type: 1})))
}

func TestSchoolAuthorizationIsCurrentAndFailsClosed(t *testing.T) {
	_, repo, mock := testStore(t)
	ctx := context.Background()
	for _, tc := range []struct {
		role, requested int
		school          *int
		allowed         bool
	}{
		{1, 0, nil, false}, {1, 20, nil, true}, {3, 0, nil, false}, {3, 20, nil, false}, {4, 20, nil, false},
		{3, 0, intPtr(10), true}, {3, 10, intPtr(10), true}, {3, 20, intPtr(10), false},
	} {
		id, err := authorizeAdmin(ctx, repo, &models.AdminUser{ID: 7, Role: tc.role, SchoolID: tc.school}, tc.requested)
		if tc.allowed {
			require.NoError(t, err)
			require.Positive(t, id)
		} else {
			require.Error(t, err)
		}
	}
	admin := &models.AdminUser{ID: 7, Role: 2, SchoolID: intPtr(99)}
	for _, requested := range []int{10, 11, 99} {
		mock.ExpectQuery("SELECT school_id FROM admin_school_relation").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"school_id"}).AddRow(10).AddRow(11))
		_, err := authorizeAdmin(ctx, repo, admin, requested)
		if requested == 99 {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}
	mock.ExpectQuery("SELECT id, username, role, school_id, status FROM admin_user").WithArgs(7).WillReturnRows(sqlmock.NewRows([]string{"id", "username", "role", "school_id", "status"}).AddRow(7, "admin", 1, nil, 0))
	_, err := Authorize(ctx, repo, 7, 10)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
func intPtr(id int) *int { return &id }

type fakeRemote struct {
	batches                         [][]RecordUpdate
	batchErr                        error
	batchFunc                       func([]RecordUpdate) error
	creates, updates                int
	deleted                         []string
	createErr, updateErr, deleteErr error
}

func (f *fakeRemote) CreateNode(context.Context, string) (string, string, error) {
	return "node", "app", nil
}
func (f *fakeRemote) CreateTable(context.Context, Target) (string, string, error) {
	return "table", "view", nil
}
func (f *fakeRemote) GetFields(context.Context, Target) ([]Field, error) { return Fields(), nil }
func (f *fakeRemote) CreateRecord(context.Context, Target, string, map[string]any) (string, error) {
	f.creates++
	return "managed-new", f.createErr
}
func (f *fakeRemote) UpdateRecord(context.Context, Target, string, map[string]any) error {
	f.updates++
	return f.updateErr
}
func (f *fakeRemote) UpdateRecords(_ context.Context, _ Target, records []RecordUpdate) error {
	f.batches = append(f.batches, append([]RecordUpdate(nil), records...))
	if f.batchFunc != nil {
		return f.batchFunc(records)
	}
	return f.batchErr
}
func (f *fakeRemote) DeleteRecord(_ context.Context, _ Target, id string) error {
	f.deleted = append(f.deleted, id)
	return f.deleteErr
}
func recordRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"user_id", "record_id", "client_token", "state"})
}

func TestExistingRecordsUpdateAndAmbiguousCreatesNeverReinsert(t *testing.T) {
	store, repo, mock := testStore(t)
	remote := &fakeRemote{}
	s := &Service{store: store, repo: repo, remote: remote}
	target := Target{SchoolID: 10}
	guard := func() error { return nil }
	ctx := context.Background()
	mock.ExpectQuery("SELECT user_id,record_id,client_token,state").WithArgs(10, 7).WillReturnRows(recordRows().AddRow(7, "managed-existing", "key", "ready"))
	mock.ExpectQuery("SELECT COUNT").WithArgs(7, 10).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	action, err := s.syncRecord(ctx, target, 7, map[string]any{"昵称": "new"}, guard)
	require.NoError(t, err)
	require.Equal(t, "updated", action)
	require.Equal(t, 1, remote.updates)
	require.Zero(t, remote.creates)
	// Save the request before calling Feishu. A lost response leaves pending state.
	remote.createErr = errors.New("lost response")
	mock.ExpectQuery("SELECT user_id,record_id,client_token,state").WithArgs(10, 8).WillReturnRows(recordRows())
	mock.ExpectQuery("SELECT COUNT").WithArgs(8, 10).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("INSERT INTO feishu_user_sync_record").WithArgs(10, 8, sqlmock.AnyArg(), `{"昵称":"new"}`).WillReturnResult(sqlmock.NewResult(1, 1))
	_, err = s.syncRecord(ctx, target, 8, map[string]any{"昵称": "new"}, guard)
	require.IsType(t, &ReconcileError{}, err)
	require.Equal(t, 1, remote.creates)
	mock.ExpectQuery("SELECT user_id,record_id,client_token,state").WithArgs(10, 8).WillReturnRows(recordRows().AddRow(8, "", "saved-key", "pending"))
	_, err = s.syncRecord(ctx, target, 8, map[string]any{"昵称": "changed"}, guard)
	require.IsType(t, &ReconcileError{}, err)
	require.Equal(t, 1, remote.creates)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRevocationAndTransferStopWrites(t *testing.T) {
	store, repo, mock := testStore(t)
	remote := &fakeRemote{}
	s := &Service{store: store, repo: repo, remote: remote}
	target := Target{SchoolID: 10}
	mock.ExpectQuery("SELECT user_id,record_id,client_token,state").WithArgs(10, 7).WillReturnRows(recordRows().AddRow(7, "managed", "key", "ready"))
	_, err := s.syncRecord(context.Background(), target, 7, nil, func() error { return &PermissionError{"revoked"} })
	require.Error(t, err)
	require.Zero(t, remote.updates)
	require.Zero(t, remote.creates)
	mock.ExpectQuery("SELECT user_id,record_id,client_token,state").WithArgs(10, 7).WillReturnRows(recordRows().AddRow(7, "managed", "key", "ready"))
	mock.ExpectQuery("SELECT COUNT").WithArgs(7, 10).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	action, err := s.syncRecord(context.Background(), target, 7, nil, func() error { return nil })
	require.NoError(t, err)
	require.Equal(t, "skipped", action)
	require.Zero(t, remote.updates)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCleanupOnlyMappedStaleRowsAndRechecksSchool(t *testing.T) {
	store, repo, mock := testStore(t)
	remote := &fakeRemote{}
	s := &Service{store: store, repo: repo, remote: remote}
	job := &Job{ID: "job", SchoolID: 10, Status: "running"}
	mock.ExpectQuery("SELECT r.user_id.*WHERE r.school_id=\\? AND u.id IS NULL").WithArgs(10).WillReturnRows(recordRows().AddRow(7, "managed-stale", "key", "ready").AddRow(8, "managed-returned", "key2", "ready"))
	mock.ExpectQuery("SELECT COUNT").WithArgs(7, 10).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("DELETE FROM feishu_user_sync_record WHERE school_id=\\? AND user_id=\\?").WithArgs(10, 7).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE feishu_user_sync_job SET status=").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COUNT").WithArgs(8, 10).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	require.NoError(t, s.cleanup(context.Background(), Target{SchoolID: 10}, job, func() error { return nil }))
	require.Equal(t, []string{"managed-stale"}, remote.deleted)
	require.Equal(t, 1, job.Deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSourceQueryAlwaysConstrainsSchoolAndKeepsMissingFields(t *testing.T) {
	store, _, mock := testStore(t)
	mock.ExpectQuery("SELECT u.id FROM.*u.school_id = \\?").WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	ids, err := store.UserIDs(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, []int{7}, ids)
	mock.ExpectQuery("SELECT u.id,COALESCE.*LEFT JOIN talent_profile.*WHERE u.school_id=\\? AND u.id=\\?").WithArgs(10, 7).WillReturnRows(sqlmock.NewRows([]string{"id", "nickname", "mbti", "school", "major", "grade", "intro", "experience", "score", "auth", "talent", "phone", "wechat", "email", "status"}).AddRow(7, "", "", "校", "", "2", "", "", nil, 0, 0, "", "", "", 0))
	row, err := store.User(context.Background(), 10, 7)
	require.NoError(t, err)
	require.Equal(t, "2", row.Grade)
	require.False(t, row.Score.Valid)
	_, err = store.UserIDs(context.Background(), 0)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDuplicateClicksReuseActiveJob(t *testing.T) {
	store, _, mock := testStore(t)
	mock.ExpectBegin()
	mock.ExpectExec("INSERT IGNORE INTO feishu_user_sync_target").WithArgs(10).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT school_id.*FOR UPDATE").WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"school_id"}).AddRow(10))
	mock.ExpectQuery("SELECT .* FROM feishu_user_sync_job WHERE active_school_id=").WithArgs(10).WillReturnRows(jobRows("existing", "queued"))
	mock.ExpectCommit()
	job, err := store.Enqueue(context.Background(), 7, 10)
	require.NoError(t, err)
	require.Equal(t, "existing", job.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}
func jobRows(id, status string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "school_id", "admin_id", "status", "total", "processed", "created_count", "updated_count", "deleted_count", "message", "created_at", "updated_at"}).AddRow(id, 10, 7, status, 2, 1, 1, 0, 0, "message", time.Now(), time.Now())
}

func TestInterruptedJobIsFailedWithoutRemoteReplay(t *testing.T) {
	store, repo, mock := testStore(t)
	remote := &fakeRemote{}
	s := &Service{repo: repo, store: store, remote: remote}
	mock.ExpectQuery("SELECT GET_LOCK").WithArgs(workerLock).WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(1))
	mock.ExpectQuery("SELECT .* FROM feishu_user_sync_job WHERE active_school_id IS NOT NULL").WillReturnRows(jobRows("interrupted", "running"))
	mock.ExpectExec("UPDATE feishu_user_sync_job SET status=\\?,active_school_id=NULL").WithArgs("failed", 2, 1, 1, 0, 0, sqlmock.AnyArg(), "interrupted").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT RELEASE_LOCK").WithArgs(workerLock).WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(1))
	require.True(t, s.workOne(context.Background()))
	require.Zero(t, remote.creates)
	require.Zero(t, remote.updates)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClientUsesBotTokenNativeValuesAndNoMutationRetry(t *testing.T) {
	creates, auths := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "tenant_access_token") {
			auths++
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, "secret", body["app_secret"])
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"test-token","expire":7200}`))
			return
		}
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		if r.Method == http.MethodPost {
			creates++
			require.Equal(t, "saved-key", r.URL.Query().Get("client_token"))
			var body struct {
				Fields map[string]any `json:"fields"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, "INTJ", body.Fields["MBTI"])
			require.Equal(t, "=raw", body.Fields["昵称"])
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"code":1255000,"msg":"secret PII must not escape"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	defer server.Close()
	client := NewClient("app", "secret", "space", "parent")
	client.baseURL = server.URL
	_, err := client.CreateRecord(context.Background(), Target{AppToken: "base", TableID: "table"}, "saved-key", map[string]any{"MBTI": "INTJ", "昵称": "=raw"})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "PII")
	require.Equal(t, 1, creates)
	require.NoError(t, client.UpdateRecord(context.Background(), Target{AppToken: "base", TableID: "table"}, "rec", map[string]any{"MBTI": nil}))
	require.Equal(t, 1, auths)
}

func TestConfirmedCreateMapsUserIDAndNextSyncUpdates(t *testing.T) {
	store, repo, mock := testStore(t)
	remote := &fakeRemote{}
	s := &Service{store: store, repo: repo, remote: remote}
	ctx := context.Background()
	target := Target{SchoolID: 10}
	guard := func() error { return nil }
	mock.ExpectQuery("SELECT user_id,record_id,client_token,state").WithArgs(10, 7).WillReturnRows(recordRows())
	mock.ExpectQuery("SELECT COUNT").WithArgs(7, 10).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("INSERT INTO feishu_user_sync_record").WithArgs(10, 7, sqlmock.AnyArg(), `{"昵称":"raw"}`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE feishu_user_sync_record SET record_id=").WithArgs("managed-new", 10, 7).WillReturnResult(sqlmock.NewResult(0, 1))
	action, err := s.syncRecord(ctx, target, 7, map[string]any{"昵称": "raw"}, guard)
	require.NoError(t, err)
	require.Equal(t, "created", action)
	mock.ExpectQuery("SELECT user_id,record_id,client_token,state").WithArgs(10, 7).WillReturnRows(recordRows().AddRow(7, "managed-new", "stored-key", "ready"))
	mock.ExpectQuery("SELECT COUNT").WithArgs(7, 10).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	action, err = s.syncRecord(ctx, target, 7, map[string]any{"昵称": "changed"}, guard)
	require.NoError(t, err)
	require.Equal(t, "updated", action)
	require.Equal(t, 1, remote.creates)
	require.Equal(t, 1, remote.updates)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMissingTableDoesNotDiscardUserMappingOrCreateDuplicates(t *testing.T) {
	store, repo, mock := testStore(t)
	remote := &fakeRemote{updateErr: &APIError{Status: 200, Code: 1254041}}
	s := &Service{store: store, repo: repo, remote: remote}
	mock.ExpectQuery("SELECT user_id,record_id,client_token,state").WithArgs(10, 7).WillReturnRows(recordRows().AddRow(7, "managed", "key", "ready"))
	mock.ExpectQuery("SELECT COUNT").WithArgs(7, 10).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	_, err := s.syncRecord(context.Background(), Target{SchoolID: 10}, 7, nil, func() error { return nil })
	require.Error(t, err)
	require.Zero(t, remote.creates)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteFailureRetainsManagedMappingForRetry(t *testing.T) {
	store, repo, mock := testStore(t)
	remote := &fakeRemote{deleteErr: errors.New("response lost")}
	s := &Service{store: store, repo: repo, remote: remote}
	job := &Job{ID: "job", SchoolID: 10, Status: "running"}
	mock.ExpectQuery("SELECT r.user_id.*WHERE r.school_id=\\? AND u.id IS NULL").WithArgs(10).WillReturnRows(recordRows().AddRow(7, "managed", "key", "ready"))
	mock.ExpectQuery("SELECT COUNT").WithArgs(7, 10).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	err := s.cleanup(context.Background(), Target{SchoolID: 10}, job, func() error { return nil })
	require.Error(t, err)
	require.Zero(t, job.Deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUnknownProvisioningResultCannotCreateAnotherBitable(t *testing.T) {
	store, repo, mock := testStore(t)
	remote := &fakeRemote{}
	s := &Service{store: store, repo: repo, remote: remote}
	mock.ExpectQuery("SELECT school_id,app_token,node_token,table_id,view_id,node_started,table_started").WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"school_id", "app_token", "node_token", "table_id", "view_id", "node_started", "table_started"}).AddRow(10, "", "", "", "", 1, 0))
	_, err := s.provision(context.Background(), &Job{SchoolID: 10}, func() error { return nil })
	require.IsType(t, &ReconcileError{}, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
