package service

import (
	"context"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
	"testing"
	"time"
)

type outcomeRepo struct {
	repository.WxSubscribeDeliveryRepo
	unknown, retry, skipped, sent, failed int
	attempt                               int
}

func (r *outcomeRepo) MarkUnknown(_ context.Context, _ int64, a int, _, _ string) error {
	r.unknown++
	r.attempt = a
	return nil
}
func (r *outcomeRepo) MarkSent(_ context.Context, _ int64, a int, _ string) error {
	r.sent++
	r.attempt = a
	return nil
}
func (r *outcomeRepo) MarkFailed(_ context.Context, _ int64, a int, _ string, _ *int, _ string) error {
	r.failed++
	r.attempt = a
	return nil
}
func (r *outcomeRepo) MarkSkipped(_ context.Context, _ int64, a int, _ string, _ int, _ string) error {
	r.skipped++
	r.attempt = a
	return nil
}
func (r *outcomeRepo) ScheduleRetry(_ context.Context, _ int64, a int, _ string, _ *int, _ string, _ time.Time) error {
	r.retry++
	r.attempt = a
	return nil
}
func TestSubscribeOutcomePolicy(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"success", nil, "sent"},
		{"transport unknown", uncertainSubscribeDeliveryError{err: context.DeadlineExceeded}, "unknown"},
		{"explicit transient", wechat.SubscribeMessageResponse{ErrCode: -1, ErrMsg: "busy"}, "retry"},
		{"user refused", wechat.SubscribeMessageResponse{ErrCode: 43101, ErrMsg: "refused"}, "skipped"},
		{"invalid template", wechat.SubscribeMessageResponse{ErrCode: 40037, ErrMsg: "invalid"}, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &outcomeRepo{}
			s := NewMessageService(&repository.Repository{WxSubscribeDelivery: r}, nil)
			s.finishSubscribeDelivery(&models.WxSubscribeDelivery{ID: 1, AttemptCount: 1}, "tpl", tc.err)
			counters := map[string]int{"sent": r.sent, "unknown": r.unknown, "retry": r.retry, "skipped": r.skipped, "failed": r.failed}
			for name, n := range counters {
				want := 0
				if name == tc.want {
					want = 1
				}
				if n != want {
					t.Fatalf("%s=%d", name, n)
				}
			}
			if r.attempt != 1 {
				t.Fatal("attempt fence missing")
			}
		})
	}
}
