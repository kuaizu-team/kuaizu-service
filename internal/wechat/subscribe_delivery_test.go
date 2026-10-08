package wechat

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSubscribeAmbiguousResponseNeverReplays(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		err    error
	}{
		{"lost response after acceptance", 0, "", errors.New("response lost")},
		{"provider timeout", 0, "", context.DeadlineExceeded},
		{"server error", 503, "", nil},
		{"redirect", 307, "", nil},
		{"truncated response", 200, `{"errcode":`, nil},
		{"empty response", 200, "", nil},
		{"missing result code", 200, `{}`, nil},
		{"null result code", 200, `{"errcode":null}`, nil},
		{"null response", 200, `null`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := NewClientWithConfig("test-app", "test-secret")
			client.token, client.tokenExp = "test-token", time.Now().Add(time.Hour)
			client.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "/cgi-bin/message/subscribe/send", req.URL.Path)
				if calls > 1 {
					return jsonResponse(`{"errcode":0}`), nil
				}
				if tc.err != nil {
					return nil, tc.err
				}
				response := jsonResponse(tc.body)
				response.StatusCode = tc.status
				if tc.status == 307 {
					response.Header.Set("Location", "https://api.weixin.qq.com/cgi-bin/message/subscribe/send")
				}
				return response, nil
			})
			err := client.SendByConfigContext(context.Background(), "test-user", "test-template", `{"remark":"thing1"}`, map[string]string{"remark": "test"}, "")
			require.Error(t, err)
			var explicit SubscribeMessageResponse
			require.False(t, errors.As(err, &explicit), "ambiguous outcome must not become an explicit rejection")
			require.Equal(t, 1, calls)
		})
	}
}

func TestSubscribeExplicitResultsAndTokenRefresh(t *testing.T) {
	for _, code := range []int{0, -1, 43101, 40037, 40001, 42001} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			client := NewClientWithConfig("test-app", "test-secret")
			client.token, client.tokenExp = "old-token", time.Now().Add(time.Hour)
			var sends, refreshes int
			client.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/cgi-bin/stable_token" {
					refreshes++
					return jsonResponse(`{"access_token":"new-token","expires_in":7200}`), nil
				}
				require.Equal(t, "/cgi-bin/message/subscribe/send", req.URL.Path)
				sends++
				if sends == 1 {
					return jsonResponse(`{"errcode":` + strconv.Itoa(code) + `,"errmsg":"test"}`), nil
				}
				require.Equal(t, "new-token", req.URL.Query().Get("access_token"))
				return jsonResponse(`{"errcode":0}`), nil
			})
			err := client.SendByConfigContext(context.Background(), "test-user", "test-template", `{"remark":"thing1"}`, map[string]string{"remark": "test"}, "")
			if isAccessTokenInvalidCode(code) {
				require.NoError(t, err)
				require.Equal(t, 2, sends)
				require.Equal(t, 1, refreshes)
			} else {
				require.Equal(t, 1, sends)
				require.Zero(t, refreshes)
				if code == 0 {
					require.NoError(t, err)
				} else {
					var explicit SubscribeMessageResponse
					require.ErrorAs(t, err, &explicit)
					require.Equal(t, code, explicit.ErrCode)
				}
			}
		})
	}
}
