package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// SubscribeMessageData 订阅消息数据字段
type SubscribeMessageData struct {
	Value string `json:"value"`
}

// SubscribeMessageRequest 订阅消息请求
type SubscribeMessageRequest struct {
	ToUser           string                          `json:"touser"`
	TemplateID       string                          `json:"template_id"`
	Page             string                          `json:"page,omitempty"`
	Data             map[string]SubscribeMessageData `json:"data"`
	MiniprogramState string                          `json:"miniprogram_state,omitempty"` // developer/trial/formal
	Lang             string                          `json:"lang,omitempty"`              // zh_CN/en_US/zh_HK/zh_TW
}

// SubscribeMessageResponse 订阅消息响应
type SubscribeMessageResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func (r SubscribeMessageResponse) Error() string {
	return fmt.Sprintf("wechat api error: %d - %s", r.ErrCode, r.ErrMsg)
}

// SendSubscribeMessage 发送订阅消息
// https://developers.weixin.qq.com/miniprogram/dev/OpenApiDoc/mp-message-management/subscribe-message/sendMessage.html
func (c *Client) SendSubscribeMessage(req *SubscribeMessageRequest) error {
	return c.SendSubscribeMessageContext(context.Background(), req)
}

type SubscribePayloadError struct {
	Err error
}

func (e SubscribePayloadError) Error() string { return e.Err.Error() }
func (e SubscribePayloadError) Unwrap() error { return e.Err }

func (c *Client) SendSubscribeMessageContext(ctx context.Context, req *SubscribeMessageRequest) error {
	if req.ToUser == "" {
		return fmt.Errorf("touser is required")
	}
	if req.TemplateID == "" {
		return fmt.Errorf("template_id is required")
	}
	if req.Data == nil || len(req.Data) == 0 {
		return fmt.Errorf("data is required")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	const maxAttempts = 3
	var result SubscribeMessageResponse
	for attempt := 0; attempt < maxAttempts; attempt++ {
		accessToken, err := c.GetAccessToken()
		if err != nil {
			return fmt.Errorf("get access token: %w", err)
		}

		url := fmt.Sprintf(
			"https://api.weixin.qq.com/cgi-bin/message/subscribe/send?access_token=%s",
			accessToken,
		)

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			if attempt+1 < maxAttempts && ctx.Err() == nil {
				if err := waitSubscribeRetry(ctx, attempt); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("send request: %w", err)
		}
		if resp.StatusCode >= http.StatusInternalServerError {
			resp.Body.Close()
			if attempt+1 < maxAttempts {
				if err := waitSubscribeRetry(ctx, attempt); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("wechat api http status: %d", resp.StatusCode)
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			resp.Body.Close()
			return fmt.Errorf("wechat api http status: %d", resp.StatusCode)
		}
		result = SubscribeMessageResponse{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return fmt.Errorf("decode response: %w", err)
		}
		resp.Body.Close()

		if isAccessTokenInvalidCode(result.ErrCode) && attempt+1 < maxAttempts {
			if _, err := c.refreshAccessToken(); err != nil {
				return fmt.Errorf("refresh access token: %w", err)
			}
			continue
		}
		break
	}

	if result.ErrCode != 0 {
		return result
	}

	return nil
}

func waitSubscribeRetry(ctx context.Context, attempt int) error {
	delay := time.Duration(attempt+1) * 200 * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("subscribe message retry canceled: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

var (
	subscribeTimePattern   = regexp.MustCompile(`^(?:[01]\d|2[0-3]):[0-5]\d$`)
	subscribeNumberPattern = regexp.MustCompile(`^-?\d+(?:\.\d+)?$`)
)

func validateSubscribeValue(templateKey, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s value is required", templateKey)
	}
	runeCount := len([]rune(value))
	switch {
	case strings.HasPrefix(templateKey, "thing"):
		if runeCount > 20 {
			return fmt.Errorf("%s exceeds 20 characters", templateKey)
		}
	case strings.HasPrefix(templateKey, "phrase"):
		if runeCount > 5 {
			return fmt.Errorf("%s exceeds 5 characters", templateKey)
		}
	case strings.HasPrefix(templateKey, "time"):
		if !subscribeTimePattern.MatchString(value) {
			return fmt.Errorf("%s must use HH:mm format", templateKey)
		}
	case strings.HasPrefix(templateKey, "number"):
		if len(value) > 32 || !subscribeNumberPattern.MatchString(value) {
			return fmt.Errorf("%s must be a number of at most 32 characters", templateKey)
		}
	}
	return nil
}

// SendByConfig sends a subscription message using a JSON mapping for fields.
// contentJSON is a map of business_key -> template_key (e.g. {"sender": "thing1"}).
// page is the miniprogram page path to navigate to when the user taps the notification card;
// an empty string means no navigation.
func (c *Client) SendByConfig(toUser string, templateID string, contentJSON string, businessData map[string]string, page string) error {
	return c.SendByConfigContext(context.Background(), toUser, templateID, contentJSON, businessData, page)
}

func (c *Client) SendByConfigContext(ctx context.Context, toUser string, templateID string, contentJSON string, businessData map[string]string, page string) error {
	var fieldMap map[string]string
	if err := json.Unmarshal([]byte(contentJSON), &fieldMap); err != nil {
		return SubscribePayloadError{Err: fmt.Errorf("unmarshal field map: %w", err)}
	}

	data := make(map[string]SubscribeMessageData)
	for bizKey, templateKey := range fieldMap {
		val, ok := businessData[bizKey]
		if !ok {
			return SubscribePayloadError{Err: fmt.Errorf("missing business data field %q", bizKey)}
		}
		if err := validateSubscribeValue(templateKey, val); err != nil {
			return SubscribePayloadError{Err: fmt.Errorf("validate business data field %q: %w", bizKey, err)}
		}
		data[templateKey] = SubscribeMessageData{Value: val}
	}

	return c.SendSubscribeMessageContext(ctx, &SubscribeMessageRequest{
		ToUser:     toUser,
		TemplateID: templateID,
		Page:       page,
		Data:       data,
	})
}
