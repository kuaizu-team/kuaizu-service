package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/messagecenter"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

type fakeUrgeStore struct {
	mu       sync.Mutex
	state    string
	token    string
	eligible bool
	failSave bool
	result   repository.AutoUrgeResult
	pages    [][]repository.AutoUrgeCandidate
	afterIDs []int
}

func (f *fakeUrgeStore) Candidates(_ context.Context, after, limit int) ([]repository.AutoUrgeCandidate, error) {
	f.afterIDs = append(f.afterIDs, after)
	if len(f.pages) == 0 {
		return nil, nil
	}
	p := f.pages[0]
	f.pages = f.pages[1:]
	return p, nil
}
func (f *fakeUrgeStore) Claim(_ context.Context, userID int, token string) (*repository.AutoUrgeClaim, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.state != "" && f.state != "failed" && f.state != "ready" {
		return nil, nil
	}
	f.state = "claimed"
	f.token = token
	return &repository.AutoUrgeClaim{UserID: userID, CycleID: 1, Token: token, RequestKey: "auto-urge:7:1"}, nil
}
func (f *fakeUrgeStore) Recheck(context.Context, int) (*repository.AutoUrgeCandidate, error) {
	if !f.eligible {
		return nil, nil
	}
	return &repository.AutoUrgeCandidate{UserID: 7, Nickname: "测试用户", PendingCount: 3}, nil
}
func (f *fakeUrgeStore) Snapshot(context.Context, repository.AutoUrgeCandidate, repository.AutoUrgeClaim) error {
	return nil
}
func (f *fakeUrgeStore) MarkDispatched(context.Context, repository.AutoUrgeClaim) error { return nil }
func (f *fakeUrgeStore) Finish(_ context.Context, claim repository.AutoUrgeClaim, r repository.AutoUrgeResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failSave {
		return errors.New("database down")
	}
	if claim.Token != f.token {
		return errors.New("wrong owner")
	}
	f.result = r
	f.state = r.State
	return nil
}

type fakeUrgeSender struct {
	mu       sync.Mutex
	calls    int
	response *messagecenter.AdminSmsSendResponse
	err      error
}

func (s *fakeUrgeSender) Send(_ context.Context, req messagecenter.AdminSmsSendRequest) (*messagecenter.AdminSmsSendResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if req.RequestKey == "" || req.TemplateKey != "URGE_PROCESS" || req.UserID != 7 || req.Variables["nickname"] != "测试用户" {
		return nil, errors.New("invalid request")
	}
	return s.response, s.err
}

func TestAutoUrgeOnlyOneConcurrentAutomaticSend(t *testing.T) {
	store := &fakeUrgeStore{eligible: true}
	sender := &fakeUrgeSender{response: &messagecenter.AdminSmsSendResponse{Success: true, RecordID: 123}}
	w := autoUrgeWorker{store: store, sender: sender}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := w.process(context.Background(), 7); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if sender.calls != 1 || store.state != "sent" || store.result.RecordID == nil || *store.result.RecordID != 123 {
		t.Fatalf("calls=%d state=%s", sender.calls, store.state)
	}
	if err := w.process(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if sender.calls != 1 {
		t.Fatal("successful user retried")
	}
}
func TestAutoUrgeOutcomePolicy(t *testing.T) {
	for _, tc := range []struct {
		name string
		resp *messagecenter.AdminSmsSendResponse
		err  error
		want string
	}{
		{"success", &messagecenter.AdminSmsSendResponse{Success: true}, nil, "sent"},
		{"explicit rejection", &messagecenter.AdminSmsSendResponse{ErrorCode: "isv.MOBILE_NUMBER_ILLEGAL"}, nil, "failed"},
		{"unrecognized provider outcome", &messagecenter.AdminSmsSendResponse{ErrorCode: "PROVIDER_ERROR"}, nil, "unknown"},
		{"provider timeout", &messagecenter.AdminSmsSendResponse{ErrorCode: "PROVIDER_TIMEOUT"}, nil, "unknown"},
		{"http timeout", nil, context.DeadlineExceeded, "unknown"},
		{"missing response", nil, nil, "unknown"},
		{"unexplained false", &messagecenter.AdminSmsSendResponse{}, nil, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyAutoUrgeResult(tc.resp, tc.err); got.State != tc.want {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
func TestAutoUrgeRecheckAndUncertainNeverRetry(t *testing.T) {
	for _, tc := range []struct {
		name               string
		eligible, failSave bool
		err                error
		state              string
		wantCalls          int
	}{
		{"already handled", false, false, nil, "ready", 0},
		{"timeout", true, false, context.DeadlineExceeded, "unknown", 1},
		{"save failed after success", true, true, nil, "claimed", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeUrgeStore{eligible: tc.eligible, failSave: tc.failSave}
			sender := &fakeUrgeSender{response: &messagecenter.AdminSmsSendResponse{Success: true}, err: tc.err}
			w := autoUrgeWorker{store: store, sender: sender}
			_ = w.process(context.Background(), 7)
			_ = w.process(context.Background(), 7)
			if store.state != tc.state || sender.calls != tc.wantCalls {
				t.Fatalf("state=%s calls=%d", store.state, sender.calls)
			}
		})
	}
}
func TestAutoUrgeExplicitFailureCanRetry(t *testing.T) {
	store := &fakeUrgeStore{eligible: true}
	sender := &fakeUrgeSender{response: &messagecenter.AdminSmsSendResponse{ErrorCode: "isv.BUSINESS_LIMIT_CONTROL"}}
	w := autoUrgeWorker{store: store, sender: sender}
	if err := w.process(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if store.state != "failed" {
		t.Fatal(store.state)
	}
	// Repository separately enforces next_retry_at; this simulates the next scan.
	sender.response = &messagecenter.AdminSmsSendResponse{Success: true}
	if err := w.process(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if sender.calls != 2 || store.state != "sent" {
		t.Fatal("retry failed")
	}
}
func TestAutoUrgeKeysetPagination(t *testing.T) {
	store := &fakeUrgeStore{pages: [][]repository.AutoUrgeCandidate{{{UserID: 7}}, {{UserID: 9}}}}
	if err := (autoUrgeWorker{store: store, sender: &fakeUrgeSender{}}).run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.afterIDs) != 3 || store.afterIDs[0] != 0 || store.afterIDs[1] != 7 || store.afterIDs[2] != 9 {
		t.Fatal(store.afterIDs)
	}
}
func TestAutoUrgeTenAMBeijing(t *testing.T) {
	for _, input := range []string{"2026-09-21T01:59:59Z", "2026-09-21T02:00:00Z", "2026-09-21T23:59:59Z"} {
		now, _ := time.Parse(time.RFC3339, input)
		next := nextAutoUrgeScan(now)
		if next.Hour() != 10 || next.Minute() != 0 || !next.After(now) || next.Sub(now) > 24*time.Hour {
			t.Fatalf("%s -> %s", now, next)
		}
	}
}

type recoveryUrgeStore struct {
	fakeUrgeStore
	rechecked, dispatched int
}

func (f *recoveryUrgeStore) Claim(_ context.Context, userID int, token string) (*repository.AutoUrgeClaim, error) {
	f.token = token
	return &repository.AutoUrgeClaim{UserID: userID, CycleID: 1, Token: token, RequestKey: "auto-urge:7:1", ReconcileOnly: true}, nil
}
func (f *recoveryUrgeStore) Recheck(context.Context, int) (*repository.AutoUrgeCandidate, error) {
	f.rechecked++
	return nil, errors.New("must not recheck for lookup")
}
func (f *recoveryUrgeStore) MarkDispatched(context.Context, repository.AutoUrgeClaim) error {
	f.dispatched++
	return errors.New("must not dispatch")
}

type captureUrgeSender struct {
	request  messagecenter.AdminSmsSendRequest
	response *messagecenter.AdminSmsSendResponse
}

func (s *captureUrgeSender) Send(_ context.Context, req messagecenter.AdminSmsSendRequest) (*messagecenter.AdminSmsSendResponse, error) {
	s.request = req
	return s.response, nil
}
func TestAutoUrgeRecoveryOnlyLooksUpExistingKey(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response *messagecenter.AdminSmsSendResponse
		state    string
	}{
		{"success persisted", &messagecenter.AdminSmsSendResponse{Success: true, RecordID: 12}, "sent"},
		{"never dispatched", &messagecenter.AdminSmsSendResponse{ErrorCode: "NOT_DISPATCHED"}, "failed"},
		{"still sending", &messagecenter.AdminSmsSendResponse{ErrorCode: "OUTCOME_UNKNOWN"}, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &recoveryUrgeStore{}
			sender := &captureUrgeSender{response: tc.response}
			err := (autoUrgeWorker{store: store, sender: sender}).process(context.Background(), 7)
			if err != nil {
				t.Fatal(err)
			}
			if !sender.request.ReconcileOnly || sender.request.RequestKey != "auto-urge:7:1" || sender.request.UserID != 7 {
				t.Fatal(sender.request)
			}
			if store.rechecked != 0 || store.dispatched != 0 || store.state != tc.state {
				t.Fatalf("state=%s recheck=%d dispatch=%d", store.state, store.rechecked, store.dispatched)
			}
		})
	}
}

type failingUserUrgeStore struct {
	fakeUrgeStore
	claimed []int
}

func (f *failingUserUrgeStore) Claim(_ context.Context, userID int, _ string) (*repository.AutoUrgeClaim, error) {
	f.claimed = append(f.claimed, userID)
	if userID == 7 {
		return nil, errors.New("bad row")
	}
	return nil, nil
}
func TestAutoUrgeUserFailureDoesNotBlockRemainingPage(t *testing.T) {
	store := &failingUserUrgeStore{fakeUrgeStore: fakeUrgeStore{pages: [][]repository.AutoUrgeCandidate{{{UserID: 7}, {UserID: 9}}}}}
	err := (autoUrgeWorker{store: store, sender: &fakeUrgeSender{}}).run(context.Background())
	if err == nil || len(store.claimed) != 2 || store.claimed[1] != 9 {
		t.Fatalf("%v %v", err, store.claimed)
	}
}
func TestAutoUrgeStartupCatchupAndCompletedDay(t *testing.T) {
	for _, tc := range []struct{ now, completed, want string }{
		{"2026-10-06T01:59:59Z", "", "2026-10-06T02:00:00Z"},
		{"2026-10-06T02:00:00Z", "", "2026-10-06T02:00:00Z"},
		{"2026-10-06T08:00:00Z", "", "2026-10-06T08:00:00Z"},
		{"2026-10-06T08:00:00Z", "2026-10-06", "2026-10-07T02:00:00Z"},
	} {
		now, _ := time.Parse(time.RFC3339, tc.now)
		want, _ := time.Parse(time.RFC3339, tc.want)
		if got := autoUrgeScanDue(now, tc.completed); !got.Equal(want) {
			t.Fatalf("%s completed=%s got=%s", tc.now, tc.completed, got)
		}
	}
}

type fencedUrgeStore struct{ fakeUrgeStore }

func (f *fencedUrgeStore) MarkDispatched(context.Context, repository.AutoUrgeClaim) error {
	return repository.ErrAutoUrgeClaimLost
}
func TestAutoUrgeClosedCycleCannotDispatch(t *testing.T) {
	store := &fencedUrgeStore{fakeUrgeStore: fakeUrgeStore{eligible: true}}
	sender := &fakeUrgeSender{}
	err := (autoUrgeWorker{store: store, sender: sender}).process(context.Background(), 7)
	if !errors.Is(err, repository.ErrAutoUrgeClaimLost) || sender.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, sender.calls)
	}
}
