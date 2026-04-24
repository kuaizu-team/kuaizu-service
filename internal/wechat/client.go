package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

// Code2SessionResponse is the response from WeChat code2session API
type Code2SessionResponse struct {
	OpenID     string `json:"openid"`
	SessionKey string `json:"session_key"`
	UnionID    string `json:"unionid,omitempty"`
	ErrCode    int    `json:"errcode,omitempty"`
	ErrMsg     string `json:"errmsg,omitempty"`
}

type accessTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	ErrCode     int    `json:"errcode,omitempty"`
	ErrMsg      string `json:"errmsg,omitempty"`
}

type PhoneInfo struct {
	PhoneNumber     string `json:"phoneNumber"`
	PurePhoneNumber string `json:"purePhoneNumber"`
	CountryCode     string `json:"countryCode"`
}

type getPhoneNumberResponse struct {
	ErrCode   int        `json:"errcode,omitempty"`
	ErrMsg    string     `json:"errmsg,omitempty"`
	PhoneInfo *PhoneInfo `json:"phone_info,omitempty"`
}

type msgSecCheckRequest struct {
	OpenID  string `json:"openid,omitempty"`
	Content string `json:"content"`
	Scene   int    `json:"scene,omitempty"`
	Version int    `json:"version,omitempty"`
}

type msgSecCheckResponse struct {
	ErrCode int    `json:"errcode,omitempty"`
	ErrMsg  string `json:"errmsg,omitempty"`
	Result  *struct {
		Suggest string `json:"suggest,omitempty"`
		Label   int    `json:"label,omitempty"`
	} `json:"result,omitempty"`
}

// ErrContentBlocked indicates WeChat rejected the submitted content.
var ErrContentBlocked = errors.New("wechat content audit rejected")

// Client is a WeChat Mini Program API client
type Client struct {
	appID      string
	appSecret  string
	httpClient *http.Client
	mu         sync.Mutex
	token      string
	tokenExp   time.Time
}

// NewClient creates a new WeChat client from environment variables
func NewClient() *Client {
	return &Client{
		appID:     os.Getenv("WECHAT_APPID"),
		appSecret: os.Getenv("WECHAT_SECRET"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NewClientWithConfig creates a new WeChat client with explicit config
func NewClientWithConfig(appID, appSecret string) *Client {
	return &Client{
		appID:     appID,
		appSecret: appSecret,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Code2Session exchanges the login code for openid and session_key
// https://developers.weixin.qq.com/miniprogram/dev/OpenApiDoc/user-login/code2Session.html
func (c *Client) Code2Session(code string) (*Code2SessionResponse, error) {
	if c.appID == "" || c.appSecret == "" {
		return nil, fmt.Errorf("WECHAT_APPID or WECHAT_SECRET not configured")
	}

	url := fmt.Sprintf(
		"https://api.weixin.qq.com/sns/jscode2session?appid=%s&secret=%s&js_code=%s&grant_type=authorization_code",
		c.appID, c.appSecret, code,
	)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("request wechat api: %w", err)
	}
	defer resp.Body.Close()

	var result Code2SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Check for WeChat API errors
	if result.ErrCode != 0 {
		return nil, fmt.Errorf("wechat api error: %d - %s", result.ErrCode, result.ErrMsg)
	}

	if result.OpenID == "" {
		return nil, fmt.Errorf("wechat api returned empty openid")
	}

	return &result, nil
}

// GetAccessToken retrieves (and caches) the WeChat access_token
// https://developers.weixin.qq.com/miniprogram/dev/OpenApiDoc/mp-access-token/getAccessToken.html
func (c *Client) GetAccessToken() (string, error) {
	if c.appID == "" || c.appSecret == "" {
		return "", fmt.Errorf("WECHAT_APPID or WECHAT_SECRET not configured")
	}

	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.tokenExp.Add(-5*time.Minute)) {
		token := c.token
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	url := fmt.Sprintf(
		"https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s",
		c.appID, c.appSecret,
	)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("request access token: %w", err)
	}
	defer resp.Body.Close()

	var result accessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode access token response: %w", err)
	}
	if result.ErrCode != 0 {
		return "", fmt.Errorf("wechat api error: %d - %s", result.ErrCode, result.ErrMsg)
	}
	if result.AccessToken == "" || result.ExpiresIn == 0 {
		return "", fmt.Errorf("wechat api returned empty access token")
	}

	exp := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	c.mu.Lock()
	c.token = result.AccessToken
	c.tokenExp = exp
	c.mu.Unlock()

	return result.AccessToken, nil
}

// GetPhoneNumber exchanges phoneCode for the user's phone number
// https://developers.weixin.qq.com/miniprogram/dev/OpenApiDoc/user-info/phone-number/getPhoneNumber.html
func (c *Client) GetPhoneNumber(code string) (string, error) {
	if code == "" {
		return "", fmt.Errorf("phone code is empty")
	}

	accessToken, err := c.GetAccessToken()
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf(
		"https://api.weixin.qq.com/wxa/business/getuserphonenumber?access_token=%s",
		accessToken,
	)
	body, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		return "", fmt.Errorf("marshal phone code: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("create phone request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request phone number: %w", err)
	}
	defer resp.Body.Close()

	var result getPhoneNumberResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode phone response: %w", err)
	}
	if result.ErrCode != 0 {
		return "", fmt.Errorf("wechat api error: %d - %s", result.ErrCode, result.ErrMsg)
	}
	if result.PhoneInfo == nil {
		return "", fmt.Errorf("wechat api returned empty phone info")
	}

	phone := result.PhoneInfo.PurePhoneNumber
	if phone == "" {
		phone = result.PhoneInfo.PhoneNumber
	}
	if phone == "" {
		return "", fmt.Errorf("wechat api returned empty phone number")
	}

	return phone, nil
}

// MsgSecCheck checks whether text content is compliant.
// https://developers.weixin.qq.com/miniprogram/dev/OpenApiDoc/sec-center/sec-check/msgSecCheck.html
func (c *Client) MsgSecCheck(ctx context.Context, openID, content string) error {
	if openID == "" {
		return fmt.Errorf("openid is empty")
	}
	if content == "" {
		return fmt.Errorf("content is empty")
	}

	accessToken, err := c.GetAccessToken()
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}

	url := fmt.Sprintf("https://api.weixin.qq.com/wxa/msg_sec_check?access_token=%s", accessToken)
	body, err := json.Marshal(msgSecCheckRequest{
		OpenID:  openID,
		Content: content,
		Scene:   1,
		Version: 2,
	})
	if err != nil {
		return fmt.Errorf("marshal msgSecCheck request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("create msgSecCheck request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request msgSecCheck: %w", err)
	}
	defer resp.Body.Close()

	var result msgSecCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode msgSecCheck response: %w", err)
	}

	if result.ErrCode != 0 {
		return fmt.Errorf("wechat api error: %d - %s", result.ErrCode, result.ErrMsg)
	}

	// 检查命中标签枚举值，100 正常；10001 广告；20001 时政；...
	if result.Result != nil && result.Result.Label > 100 {
		return ErrContentBlocked
	}

	return nil
}
