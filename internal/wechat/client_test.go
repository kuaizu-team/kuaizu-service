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
			case "/cgi-bin/stable_token":
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				assert.JSONEq(t, `{
					"grant_type":"client_credential",
					"appid":"test-appid",
					"secret":"test-secret"
				}`, string(body))

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
			case "/cgi-bin/stable_token":
				return jsonResponse(`{"access_token":"token-123","expires_in":7200}`), nil
			case "/wxa/msg_sec_check":
				return jsonResponse(`{"errcode":0,"errmsg":"ok","result":{"suggest":"risky","label":10001}}`), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	err := client.MsgSecCheck(context.Background(), "openid-123", "hello world")
	require.ErrorIs(t, err, ErrContentBlocked)
}

func TestGetPhoneNumberRefreshesTokenOnInvalidCredential(t *testing.T) {
	client := NewClientWithConfig("test-appid", "test-secret")
	var tokenReqs int
	var phoneReqs int
	client.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/cgi-bin/stable_token":
				tokenReqs++
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				if tokenReqs == 1 {
					assert.JSONEq(t, `{
						"grant_type":"client_credential",
						"appid":"test-appid",
						"secret":"test-secret"
					}`, string(body))
					return jsonResponse(`{"access_token":"old-token","expires_in":7200}`), nil
				}

				assert.JSONEq(t, `{
					"grant_type":"client_credential",
					"appid":"test-appid",
					"secret":"test-secret",
					"force_refresh":true
				}`, string(body))
				return jsonResponse(`{"access_token":"new-token","expires_in":7200}`), nil
			case "/wxa/business/getuserphonenumber":
				phoneReqs++
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				assert.JSONEq(t, `{"code":"phone-code-123"}`, string(body))

				if phoneReqs == 1 {
					assert.Equal(t, "old-token", req.URL.Query().Get("access_token"))
					return jsonResponse(`{"errcode":40001,"errmsg":"invalid credential"}`), nil
				}

				assert.Equal(t, "new-token", req.URL.Query().Get("access_token"))
				return jsonResponse(`{
					"errcode":0,
					"errmsg":"ok",
					"phone_info":{"purePhoneNumber":"13800138000"}
				}`), nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	phone, err := client.GetPhoneNumber("phone-code-123")
	require.NoError(t, err)
	assert.Equal(t, "13800138000", phone)
	assert.Equal(t, 2, tokenReqs)
	assert.Equal(t, 2, phoneReqs)
}
