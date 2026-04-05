package wechat

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestMsgSecCheckIncludesOpenID(t *testing.T) {
	client := NewClientWithConfig("test-appid", "test-secret")
	client.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/cgi-bin/token":
				return jsonResponse(`{"access_token":"token-123","expires_in":7200}`), nil
			case "/wxa/msg_sec_check":
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)

				assert.JSONEq(t, `{
					"openid":"openid-123",
					"content":"hello world",
					"scene":1,
					"version":2
				}`, string(body))

				return jsonResponse(`{"errcode":0,"errmsg":"ok","result":{"suggest":"pass","label":0}}`), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	err := client.MsgSecCheck(context.Background(), "openid-123", "hello world")
	require.NoError(t, err)
}

func TestMsgSecCheckReturnsBlockedError(t *testing.T) {
	client := NewClientWithConfig("test-appid", "test-secret")
	client.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/cgi-bin/token":
				return jsonResponse(`{"access_token":"token-123","expires_in":7200}`), nil
			case "/wxa/msg_sec_check":
				return jsonResponse(`{"errcode":0,"errmsg":"ok","result":{"suggest":"risky","label":100}}`), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	err := client.MsgSecCheck(context.Background(), "openid-123", "hello world")
	require.ErrorIs(t, err, ErrContentBlocked)
}
