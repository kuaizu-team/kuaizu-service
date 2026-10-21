package feishusync

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func pageArgs(ids []int) []driver.Value {
	args := []driver.Value{10}
	for _, id := range ids {
		args = append(args, id)
	}
	return args
}
func expectMappings(mock sqlmock.Sqlmock, ids []int, existing []int) {
	rows := recordRows()
	for _, id := range existing {
		rows.AddRow(id, fmt.Sprintf("rec-%d", id), "key", "ready")
	}
	mock.ExpectQuery("SELECT user_id,record_id,client_token,state FROM feishu_user_sync_record WHERE school_id=.*AND user_id IN").WithArgs(pageArgs(ids)...).WillReturnRows(rows)
}
func expectUser(mock sqlmock.Sqlmock, id int, intro, experience string) {
	mock.ExpectQuery("SELECT u.id,COALESCE.*WHERE u.school_id=.*AND u.id=").WithArgs(10, id).WillReturnRows(
		sqlmock.NewRows([]string{"id", "nickname", "mbti", "school", "major", "grade", "intro", "experience", "score", "auth", "talent", "phone", "wechat", "email", "status"}).AddRow(id, fmt.Sprintf("用户%d", id), "", "学校10", "", "2", intro, experience, nil, 0, 0, "00123", "", "", 0))
}
func expectBelonging(mock sqlmock.Sqlmock, ids, eligible []int) {
	rows := sqlmock.NewRows([]string{"id"})
	for _, id := range eligible {
		rows.AddRow(id)
	}
	mock.ExpectQuery("SELECT id FROM `user` WHERE school_id=.*AND id IN").WithArgs(pageArgs(ids)...).WillReturnRows(rows)
}
func expectProgress(mock sqlmock.Sqlmock) {
	mock.ExpectExec("UPDATE feishu_user_sync_job SET status=").WillReturnResult(sqlmock.NewResult(0, 1))
}
func testJob(total int) *Job { return &Job{ID: "job", SchoolID: 10, Status: "running", Total: total} }
func updatesFor(ids ...int) []pendingUpdate {
	updates := make([]pendingUpdate, 0, len(ids))
	for _, id := range ids {
		updates = append(updates, pendingUpdate{userID: id, record: RecordUpdate{RecordID: fmt.Sprintf("rec-%d", id), Fields: map[string]any{"昵称": "最新内容", "MBTI": nil}}})
	}
	return updates
}

func TestRepeatSync38UsersUsesOneBatchAndPreservesAll15Fields(t *testing.T) {
	store, repo, mock := testStore(t)
	remote := &fakeRemote{}
	s := &Service{store: store, repo: repo, remote: remote}
	ids := make([]int, 38)
	for i := range ids {
		ids[i] = i + 1
	}
	expectMappings(mock, ids, ids)
	for _, id := range ids {
		expectUser(mock, id, "第一行\n第二行", "=原始内容")
	}
	expectBelonging(mock, ids, ids)
	expectProgress(mock)
	expectProgress(mock)
	job := testJob(len(ids))
	require.NoError(t, s.syncPage(context.Background(), Target{SchoolID: 10}, job, ids, func() error { return nil }))
	require.Len(t, remote.batches, 1)
	require.Len(t, remote.batches[0], 38)
	for _, row := range remote.batches[0] {
		require.Len(t, row.Fields, 15)
		require.Equal(t, "第一行\n第二行", row.Fields["自我介绍"])
		require.Equal(t, "=原始内容", row.Fields["项目经历"])
		require.Equal(t, "00123", row.Fields["电话"])
		require.Nil(t, row.Fields["MBTI"])
		require.NotContains(t, row.Fields, "用户ID")
	}
	require.Equal(t, 38, job.Updated)
	require.Equal(t, 38, job.Processed)
	require.Zero(t, remote.creates)
	require.Zero(t, remote.updates)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchoolSyncSplits51UsersInto50And1(t *testing.T) {
	store, repo, mock := testStore(t)
	remote := &fakeRemote{}
	s := &Service{store: store, repo: repo, remote: remote}
	target := Target{SchoolID: 10, AppToken: "base", NodeToken: "node", TableID: "table", ViewID: "view"}
	mock.ExpectQuery("SELECT school_id,app_token.*FROM feishu_user_sync_target").WithArgs(10).WillReturnRows(sqlmock.NewRows([]string{"school_id", "app_token", "node_token", "table_id", "view_id", "node_started", "table_started"}).AddRow(10, "base", "node", "table", "view", true, true))
	mock.ExpectQuery("SELECT r.user_id.*WHERE r.school_id=.*AND u.id IS NULL").WithArgs(10).WillReturnRows(recordRows())
	ids := make([]int, 51)
	idRows := sqlmock.NewRows([]string{"id"})
	for i := range ids {
		ids[i] = i + 1
		idRows.AddRow(ids[i])
	}
	mock.ExpectQuery("SELECT u.id FROM.*ORDER BY u.id").WithArgs(10).WillReturnRows(idRows)
	expectProgress(mock)
	for _, page := range [][]int{ids[:50], ids[50:]} {
		expectMappings(mock, page, page)
		for _, id := range page {
			expectUser(mock, id, "", "")
		}
		expectBelonging(mock, page, page)
		expectProgress(mock)
		expectProgress(mock)
	}
	mock.ExpectQuery("SELECT r.user_id.*WHERE r.school_id=.*AND u.id IS NULL").WithArgs(10).WillReturnRows(recordRows())
	job := testJob(0)
	require.NoError(t, s.syncSchool(context.Background(), job, func() error { return nil }))
	require.Len(t, remote.batches, 2)
	require.Len(t, remote.batches[0], 50)
	require.Len(t, remote.batches[1], 1)
	require.Equal(t, target.SchoolID, job.SchoolID)
	require.Equal(t, 51, job.Total)
	require.Equal(t, 51, job.Processed)
	require.Equal(t, 51, job.Updated)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMixedPageBatchesExistingAndCreatesNewIndividually(t *testing.T) {
	store, repo, mock := testStore(t)
	remote := &fakeRemote{}
	s := &Service{store: store, repo: repo, remote: remote}
	expectMappings(mock, []int{1, 2, 3}, []int{1, 2})
	expectUser(mock, 1, "", "")
	expectUser(mock, 2, "", "")
	expectUser(mock, 3, "", "")
	expectBelonging(mock, []int{1, 2}, []int{1, 2})
	expectProgress(mock)
	mock.ExpectQuery("SELECT user_id,record_id,client_token,state").WithArgs(10, 3).WillReturnRows(recordRows())
	mock.ExpectQuery("SELECT COUNT").WithArgs(3, 10).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("INSERT INTO feishu_user_sync_record").WithArgs(10, 3, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE feishu_user_sync_record SET record_id=").WithArgs("managed-new", 10, 3).WillReturnResult(sqlmock.NewResult(0, 1))
	expectProgress(mock)
	expectProgress(mock)
	job := testJob(3)
	require.NoError(t, s.syncPage(context.Background(), Target{SchoolID: 10}, job, []int{1, 2, 3}, func() error { return nil }))
	require.Len(t, remote.batches, 1)
	require.Len(t, remote.batches[0], 2)
	require.Equal(t, 1, remote.creates)
	require.Equal(t, 2, job.Updated)
	require.Equal(t, 1, job.Created)
	require.Equal(t, 3, job.Processed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchByteBudgetAndOversizedSingleRow(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		t.Run(fmt.Sprint(oversized), func(t *testing.T) {
			store, repo, mock := testStore(t)
			remote := &fakeRemote{}
			s := &Service{store: store, repo: repo, remote: remote}
			text := strings.Repeat("中", 100000)
			ids := []int{1, 2}
			if oversized {
				text = strings.Repeat("\x00", 100000)
				ids = []int{1}
			}
			expectMappings(mock, ids, ids)
			for i, id := range ids {
				expectUser(mock, id, text, text)
				if oversized {
					mock.ExpectQuery("SELECT user_id,record_id,client_token,state").WithArgs(10, id).WillReturnRows(recordRows().AddRow(id, "rec-1", "key", "ready"))
					mock.ExpectQuery("SELECT COUNT").WithArgs(id, 10).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
					expectProgress(mock)
				} else if i == 1 {
					// The second row exceeds the byte budget and flushes the first.
					expectBelonging(mock, []int{1}, []int{1})
					expectProgress(mock)
				}
			}
			if !oversized {
				expectBelonging(mock, []int{2}, []int{2})
				expectProgress(mock)
			}
			expectProgress(mock)
			job := testJob(len(ids))
			require.NoError(t, s.syncPage(context.Background(), Target{SchoolID: 10}, job, ids, func() error { return nil }))
			if oversized {
				require.Empty(t, remote.batches)
				require.Equal(t, 1, remote.updates)
			} else {
				require.Len(t, remote.batches, 2)
				for _, batch := range remote.batches {
					require.Len(t, batch, 1)
				}
			}
			require.Equal(t, len(ids), job.Updated)
			require.Equal(t, len(ids), job.Processed)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestBatchFiltersTransferredUsersImmediatelyBeforeWriting(t *testing.T) {
	store, repo, mock := testStore(t)
	remote := &fakeRemote{}
	s := &Service{store: store, repo: repo, remote: remote}
	expectBelonging(mock, []int{1, 2}, []int{2})
	expectProgress(mock)
	job := testJob(2)
	require.NoError(t, s.flushUpdates(context.Background(), Target{SchoolID: 10}, job, updatesFor(1, 2), func() error { return nil }))
	require.Len(t, remote.batches, 1)
	require.Len(t, remote.batches[0], 1)
	require.Equal(t, "rec-2", remote.batches[0][0].RecordID)
	require.Equal(t, 2, job.Processed)
	require.Equal(t, 1, job.Updated)
	require.Empty(t, remote.deleted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchRechecksPermissionAfterSchoolQuery(t *testing.T) {
	store, repo, mock := testStore(t)
	remote := &fakeRemote{}
	s := &Service{store: store, repo: repo, remote: remote}
	expectBelonging(mock, []int{1}, []int{1})
	guards := 0
	guard := func() error {
		guards++
		if guards == 2 {
			return &PermissionError{"revoked"}
		}
		return nil
	}
	job := testJob(1)
	require.Error(t, s.flushUpdates(context.Background(), Target{SchoolID: 10}, job, updatesFor(1), guard))
	require.Empty(t, remote.batches)
	require.Zero(t, job.Processed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchUncertainOrPermissionFailureNeverCreatesOrForgetsMappings(t *testing.T) {
	for _, failure := range []error{errors.New("timeout"), errors.New("incomplete response"), &APIError{Status: 403, Code: 1254302}, &APIError{Status: 200, Code: 1254041}} {
		t.Run(failure.Error(), func(t *testing.T) {
			store, repo, mock := testStore(t)
			remote := &fakeRemote{batchErr: failure}
			s := &Service{store: store, repo: repo, remote: remote}
			expectBelonging(mock, []int{1, 2}, []int{1, 2})
			job := testJob(2)
			require.Error(t, s.flushUpdates(context.Background(), Target{SchoolID: 10}, job, updatesFor(1, 2), func() error { return nil }))
			require.Len(t, remote.batches, 1)
			require.Zero(t, remote.creates)
			require.Zero(t, remote.updates)
			require.Zero(t, job.Processed)
			require.Zero(t, job.Updated)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestMissingBatchRecordIsIsolatedAndConfirmedBeforeReplacement(t *testing.T) {
	store, repo, mock := testStore(t)
	missing := &APIError{Status: 200, Code: 1254043}
	remote := &fakeRemote{updateErr: missing, batchFunc: func(records []RecordUpdate) error {
		for _, record := range records {
			if record.RecordID == "rec-2" {
				return missing
			}
		}
		return nil
	}}
	s := &Service{store: store, repo: repo, remote: remote}
	expectBelonging(mock, []int{1, 2}, []int{1, 2})
	expectBelonging(mock, []int{1}, []int{1})
	expectProgress(mock)
	expectBelonging(mock, []int{2}, []int{2})
	mock.ExpectQuery("SELECT user_id,record_id,client_token,state").WithArgs(10, 2).WillReturnRows(recordRows().AddRow(2, "rec-2", "key", "ready"))
	mock.ExpectQuery("SELECT COUNT").WithArgs(2, 10).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("DELETE FROM feishu_user_sync_record WHERE school_id=.*AND user_id=").WithArgs(10, 2).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COUNT").WithArgs(2, 10).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("INSERT INTO feishu_user_sync_record").WithArgs(10, 2, sqlmock.AnyArg(), `{"MBTI":null,"昵称":"最新内容"}`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE feishu_user_sync_record SET record_id=").WithArgs("managed-new", 10, 2).WillReturnResult(sqlmock.NewResult(0, 1))
	expectProgress(mock)
	job := testJob(2)
	require.NoError(t, s.flushUpdates(context.Background(), Target{SchoolID: 10}, job, updatesFor(1, 2), func() error { return nil }))
	require.Len(t, remote.batches, 3)
	require.Equal(t, 1, remote.creates)
	require.Equal(t, 1, remote.updates)
	require.Equal(t, 1, job.Created)
	require.Equal(t, 1, job.Updated)
	require.Equal(t, 2, job.Processed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClientBatchUsesNativeContractAndRequiresEveryAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		valid          bool
	}{
		{"reordered", `{"code":0,"data":{"records":[{"record_id":"rec-2"},{"record_id":"rec-1"}]}}`, true},
		{"incomplete", `{"code":0,"data":{"records":[{"record_id":"rec-1"}]}}`, false},
		{"duplicate", `{"code":0,"data":{"records":[{"record_id":"rec-1"},{"record_id":"rec-1"}]}}`, false},
		{"unexpected", `{"code":0,"data":{"records":[{"record_id":"rec-x"},{"record_id":"rec-1"}]}}`, false},
		{"empty", `{"code":0,"data":{}}`, false},
		{"api-error", `{"code":1254043,"msg":"private data"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/open-apis/bitable/v1/apps/base/tables/table/records/batch_update", r.URL.Path)
				require.Empty(t, r.URL.RawQuery)
				require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
				var body struct {
					Records []RecordUpdate `json:"records"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Len(t, body.Records, 2)
				require.Equal(t, "=原始内容\n第二行", body.Records[0].Fields["昵称"])
				require.Nil(t, body.Records[0].Fields["MBTI"])
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tc.response)
			}))
			defer server.Close()
			client := NewClient("app", "secret", "space", "parent")
			client.baseURL = server.URL
			client.token = "test-token"
			client.expires = time.Now().Add(time.Hour)
			err := client.UpdateRecords(context.Background(), Target{AppToken: "base", TableID: "table"}, []RecordUpdate{{RecordID: "rec-1", Fields: map[string]any{"昵称": "=原始内容\n第二行", "MBTI": nil}}, {RecordID: "rec-2", Fields: map[string]any{"昵称": "最新"}}})
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "private data")
			}
			require.Equal(t, 1, calls)
		})
	}
}

func TestClientRejectsInvalidBatchBeforeAnyRemoteRequest(t *testing.T) {
	client := NewClient("app", "secret", "space", "parent")
	for _, records := range [][]RecordUpdate{nil, {{RecordID: ""}}, {{RecordID: "same"}, {RecordID: "same"}}, make([]RecordUpdate, 51), {{RecordID: "rec", Fields: map[string]any{"昵称": strings.Repeat("x", updateBatchBytes)}}}} {
		require.Error(t, client.UpdateRecords(context.Background(), Target{}, records))
	}
}

func TestPageWithUnconfirmedCreateDoesNotSendBufferedUpdatesOrReinsert(t *testing.T) {
	store, repo, mock := testStore(t)
	remote := &fakeRemote{}
	s := &Service{store: store, repo: repo, remote: remote}
	mock.ExpectQuery("SELECT user_id,record_id,client_token,state FROM feishu_user_sync_record WHERE school_id=.*AND user_id IN").WithArgs(10, 1, 2).WillReturnRows(recordRows().AddRow(1, "rec-1", "key", "ready").AddRow(2, "", "key2", "pending"))
	expectUser(mock, 1, "", "")
	expectUser(mock, 2, "", "")
	job := testJob(2)
	err := s.syncPage(context.Background(), Target{SchoolID: 10}, job, []int{1, 2}, func() error { return nil })
	require.IsType(t, &ReconcileError{}, err)
	require.Empty(t, remote.batches)
	require.Zero(t, remote.creates)
	require.Zero(t, job.Processed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMissingBatchWithUncertainSingleUpdateRetainsMappingAndConfirmedProgress(t *testing.T) {
	store, repo, mock := testStore(t)
	missing := &APIError{Status: 200, Code: 1254043}
	remote := &fakeRemote{updateErr: errors.New("lost response"), batchFunc: func(records []RecordUpdate) error {
		for _, record := range records {
			if record.RecordID == "rec-2" {
				return missing
			}
		}
		return nil
	}}
	s := &Service{store: store, repo: repo, remote: remote}
	expectBelonging(mock, []int{1, 2}, []int{1, 2})
	expectBelonging(mock, []int{1}, []int{1})
	expectProgress(mock)
	expectBelonging(mock, []int{2}, []int{2})
	mock.ExpectQuery("SELECT user_id,record_id,client_token,state").WithArgs(10, 2).WillReturnRows(recordRows().AddRow(2, "rec-2", "key", "ready"))
	mock.ExpectQuery("SELECT COUNT").WithArgs(2, 10).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	job := testJob(2)
	require.Error(t, s.flushUpdates(context.Background(), Target{SchoolID: 10}, job, updatesFor(1, 2), func() error { return nil }))
	require.Zero(t, remote.creates)
	require.Equal(t, 1, job.Updated)
	require.Equal(t, 1, job.Processed)
	require.NoError(t, mock.ExpectationsWereMet())
}
