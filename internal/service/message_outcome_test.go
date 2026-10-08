package service

import (
	"context"
	"errors"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type outcomeRepo struct {
	repository.WxSubscribeDeliveryRepo
	unknown, retry, skipped, sent, failed int
	attempt                               int
}

type deliveryPolicyTransport func(*http.Request) (*http.Response, error)

func (f deliveryPolicyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type deliveryPolicyUserRepo struct{ repository.UserRepo }

func (*deliveryPolicyUserRepo) GetByID(context.Context, int) (*models.User, error) {
	return &models.User{ID: 7, OpenID: "test-user"}, nil
}

type deliveryPolicyTemplateRepo struct {
	repository.MsgTemplateConfigRepo
}

func (*deliveryPolicyTemplateRepo) GetByBizKey(context.Context, string) (*models.MsgTemplateConfig, error) {
	return &models.MsgTemplateConfig{TemplateID: "test-template", ContentJSON: `{"remark":"thing1"}`}, nil
}

type deliveryPolicyRepo struct {
	outcomeRepo
	dispatches int
}

func (r *deliveryPolicyRepo) BeginDispatch(context.Context, int64, int) (bool, error) {
	r.dispatches++
	return true, nil
}

func TestRealSubscribeClientPreservesDeliveryOutcomeBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
		err              error
	}{
		{"success", `{"errcode":0}`, "sent", 200, nil},
		{"response lost after acceptance", "", "unknown", 0, errors.New("response lost")},
		{"HTTP 503", "", "unknown", 503, nil},
		{"missing result code", `{}`, "unknown", 200, nil},
		{"explicit busy", `{"errcode":-1,"errmsg":"busy"}`, "retry", 200, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			originalTransport := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = originalTransport })
			http.DefaultTransport = deliveryPolicyTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/cgi-bin/stable_token" {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"access_token":"test-token","expires_in":7200}`)), Header: make(http.Header)}, nil
				}
				if req.URL.Path != "/cgi-bin/message/subscribe/send" {
					t.Fatalf("unexpected provider path: %s", req.URL.Path)
				}
				calls++
				if tc.err != nil {
					return nil, tc.err
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})
			r := &deliveryPolicyRepo{}
			s := NewMessageService(&repository.Repository{
				User: &deliveryPolicyUserRepo{}, MsgTemplate: &deliveryPolicyTemplateRepo{}, WxSubscribeDelivery: r,
			}, wechat.NewClientWithConfig("test-app", "test-secret"))
			delivery := &models.WxSubscribeDelivery{ID: 1, UserID: 7, AttemptCount: 1}
			template, err := s.deliverSubscribeMsg(context.Background(), 7, "test-biz", map[string]string{"remark": "test"}, "", delivery)
			s.finishSubscribeDelivery(delivery, template, err)
			if calls != 1 || r.dispatches != 1 || r.attempt != 1 {
				t.Fatalf("provider calls=%d dispatches=%d attempt=%d", calls, r.dispatches, r.attempt)
			}
			for name, count := range map[string]int{"sent": r.sent, "unknown": r.unknown, "retry": r.retry, "failed": r.failed, "skipped": r.skipped} {
				want := 0
				if name == tc.want {
					want = 1
				}
				if count != want {
					t.Fatalf("%s=%d, want %d", name, count, want)
				}
			}
		})
	}
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
