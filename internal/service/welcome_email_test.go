package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/messagecenter"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

func TestWelcomeEmailTraceIDUsesCommittedDelivery(t *testing.T) {
	if got := welcomeEmailTraceID(42); got != "welcome-email:42" {
		t.Fatalf("welcomeEmailTraceID(42) = %q", got)
	}
}

type welcomeEmailRepoStub struct {
	pending []repository.PendingWelcomeEmailDelivery
	sent    chan int64
}

func (r *welcomeEmailRepoStub) Create(context.Context, int, string) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *welcomeEmailRepoStub) ListPendingBefore(context.Context, time.Time, int) ([]repository.PendingWelcomeEmailDelivery, error) {
	return r.pending, nil
}

func (r *welcomeEmailRepoStub) ClaimPending(context.Context, int64, time.Time) (bool, error) {
	return true, nil
}

func (r *welcomeEmailRepoStub) MarkSent(_ context.Context, deliveryID int64, _ *int64) error {
	r.sent <- deliveryID
	return nil
}

func (r *welcomeEmailRepoStub) MarkFailed(context.Context, int64, string) error {
	return errors.New("unexpected failure")
}

func TestWelcomeEmailRecoverPendingUsesStableTraceID(t *testing.T) {
	requests := make(chan messagecenter.WelcomeEmailRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/email/welcome" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req messagecenter.WelcomeEmailRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requests <- req
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"success","data":{"success":true,"taskId":42}}`))
	}))
	defer server.Close()

	repo := &welcomeEmailRepoStub{
		pending: []repository.PendingWelcomeEmailDelivery{{
			ID: 17, UserID: 1130, Email: "tester@example.com", Nickname: "测试同学",
		}},
		sent: make(chan int64, 1),
	}
	svc := NewWelcomeEmailService(repo, messagecenter.NewClient(server.URL, "test-token", time.Second), nil)
	svc.recoverPending(context.Background())

	select {
	case req := <-requests:
		if req.TraceID != "welcome-email:17" {
			t.Fatalf("unexpected trace ID: %s", req.TraceID)
		}
		if req.Email != "tester@example.com" || req.Nickname != "测试同学" {
			t.Fatalf("unexpected request: %+v", req)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recovered welcome-email request")
	}

	select {
	case deliveryID := <-repo.sent:
		if deliveryID != 17 {
			t.Fatalf("unexpected delivery ID: %d", deliveryID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recovered delivery to be marked sent")
	}
}
