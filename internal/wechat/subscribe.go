package wechat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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

	var result SubscribeMessageResponse
	for attempt := 0; attempt < 2; attempt++ {
		accessToken, err := c.GetAccessToken()
		if err != nil {
			return fmt.Errorf("get access token: %w", err)
		}

		url := fmt.Sprintf(
			"https://api.weixin.qq.com/cgi-bin/message/subscribe/send?access_token=%s",
			accessToken,
		)

		httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return fmt.Errorf("decode response: %w", err)
		}
		resp.Body.Close()

		if isAccessTokenInvalidCode(result.ErrCode) && attempt == 0 {
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

// SendByConfig sends a subscription message using a JSON mapping for fields.
// contentJSON is a map of business_key -> template_key (e.g. {"sender": "thing1"})
func (c *Client) SendByConfig(toUser string, templateID string, contentJSON string, businessData map[string]string) error {
	var fieldMap map[string]string
	if err := json.Unmarshal([]byte(contentJSON), &fieldMap); err != nil {
		return fmt.Errorf("unmarshal field map: %w", err)
	}

	data := make(map[string]SubscribeMessageData)
	for bizKey, templateKey := range fieldMap {
		if val, ok := businessData[bizKey]; ok {
			data[templateKey] = SubscribeMessageData{Value: val}
		}
	}

	return c.SendSubscribeMessage(&SubscribeMessageRequest{
		ToUser:     toUser,
		TemplateID: templateID,
		Data:       data,
	})
}
