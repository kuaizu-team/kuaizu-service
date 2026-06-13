package messagecenter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultTimeout = 3 * time.Second

// Client submits marketing tasks to the independent message center.
type Client struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
}

// ProjectPromotionRequest is the message-center project promotion payload.
type ProjectPromotionRequest struct {
	PromotionID      int    `json:"promotionId,omitempty"`
	ProjectID        int    `json:"projectId"`
	PromotionCount   int    `json:"promotionCount"`
	CreatorUserID    int    `json:"creatorUserId"`
	OrderID          int    `json:"orderId"`
	Strategy         string `json:"strategy,omitempty"`
	TraceID          string `json:"traceId,omitempty"`
	RecipientUserIDs []int  `json:"recipientUserIds,omitempty"`
}

// ProjectPromotionResponse is the useful data returned by the message center.
type ProjectPromotionResponse struct {
	TaskID         string `json:"taskId"`
	PromotionID    int    `json:"promotionId"`
	RequestedCount int    `json:"requestedCount"`
	ActualCount    int    `json:"actualCount"`
}

type projectPromotionEnvelope struct {
	Code    int                       `json:"code"`
	Message string                    `json:"message"`
	Data    *ProjectPromotionResponse `json:"data"`
}

type SmsNoticeRequest struct {
	TraceID             string `json:"traceId"`
	NoticeID            int    `json:"noticeId"`
	OrderID             int    `json:"orderId"`
	SenderUserID        int    `json:"senderUserId"`
	ReceiverUserID      int    `json:"receiverUserId"`
	OliveBranchRecordID int    `json:"oliveBranchRecordId"`
	ProjectID           *int   `json:"projectId,omitempty"`
	Content             string `json:"content"`
}

type SmsNoticeResponse struct {
	Provider      string `json:"provider,omitempty"`
	ProviderBizID string `json:"providerBizId,omitempty"`
	TaskID        string `json:"taskId,omitempty"`
	Accepted      *bool  `json:"accepted,omitempty"`
}

type RegisterInviteSmsRequest struct {
	TraceID      string `json:"traceId"`
	RecordID     int    `json:"recordId"`
	Phone        string `json:"phone"`
	ProjectID    int    `json:"projectId"`
	ProjectTitle string `json:"projectTitle"`
	TeamRole     string `json:"teamRole"`
}

type RegisterInviteSmsResponse struct {
	Provider      string `json:"provider,omitempty"`
	ProviderBizID string `json:"providerBizId,omitempty"`
	TaskID        string `json:"taskId,omitempty"`
	Accepted      *bool  `json:"accepted,omitempty"`
}

type smsNoticeEnvelope struct {
	Code    int                `json:"code"`
	Message string             `json:"message"`
	Data    *SmsNoticeResponse `json:"data"`
}

type registerInviteSmsEnvelope struct {
	Code    int                        `json:"code"`
	Message string                     `json:"message"`
	Data    *RegisterInviteSmsResponse `json:"data"`
}

type AdminSmsSendRequest struct {
	TemplateKey string                 `json:"template_key"`
	UserID      int                    `json:"user_id"`
	Variables   map[string]interface{} `json:"variables,omitempty"`
}

type AdminSmsSendResponse struct {
	Success       bool   `json:"success"`
	TemplateKey   string `json:"template_key"`
	UserID        int    `json:"user_id"`
	RecordID      int64  `json:"record_id"`
	Provider      string `json:"provider,omitempty"`
	ProviderBizID string `json:"provider_biz_id,omitempty"`
	ErrorCode     string `json:"error_code,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
}

type AdminSmsSendCountResponse struct {
	UserID      int    `json:"user_id"`
	TemplateKey string `json:"template_key"`
	Days        int    `json:"days"`
	Count       int64  `json:"count"`
}

type adminSmsSendEnvelope struct {
	Code    int                   `json:"code"`
	Message string                `json:"message"`
	Data    *AdminSmsSendResponse `json:"data"`
}

type adminSmsSendCountEnvelope struct {
	Code    int                        `json:"code"`
	Message string                     `json:"message"`
	Data    *AdminSmsSendCountResponse `json:"data"`
}

// NewClientFromEnv builds a message-center client from environment variables.
func NewClientFromEnv() (*Client, error) {
	baseURL := strings.TrimSpace(os.Getenv("MESSAGE_CENTER_BASE_URL"))
	if baseURL == "" {
		return nil, fmt.Errorf("MESSAGE_CENTER_BASE_URL is required")
	}

	apiToken := strings.TrimSpace(os.Getenv("MESSAGE_CENTER_API_TOKEN"))
	if apiToken == "" {
		return nil, fmt.Errorf("MESSAGE_CENTER_API_TOKEN is required")
	}

	return NewClient(baseURL, apiToken, defaultTimeout), nil
}

// NewClient creates a message-center client.
func NewClient(baseURL, apiToken string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiToken: strings.TrimSpace(apiToken),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// BaseURL returns the configured message-center base URL for diagnostics.
func (c *Client) BaseURL() string {
	if c == nil {
		return ""
	}
	return c.baseURL
}

// SubmitProjectPromotion submits a project promotion task.
func (c *Client) SubmitProjectPromotion(ctx context.Context, req ProjectPromotionRequest) (*ProjectPromotionResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("message center client is nil")
	}
	if req.OrderID <= 0 {
		return nil, fmt.Errorf("orderId is required")
	}
	if req.ProjectID <= 0 {
		return nil, fmt.Errorf("projectId is required")
	}
	if req.PromotionCount <= 0 {
		return nil, fmt.Errorf("promotionCount is required")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/api/v2/marketing/project-promotion",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("post project promotion: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var envelope projectPromotionEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if envelope.Code != http.StatusOK {
		return nil, fmt.Errorf("message center code %d: %s", envelope.Code, envelope.Message)
	}
	if envelope.Data == nil {
		return nil, fmt.Errorf("message center response data is empty")
	}

	return envelope.Data, nil
}

func (c *Client) SubmitSmsNotice(ctx context.Context, req SmsNoticeRequest) (*SmsNoticeResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("message center client is nil")
	}
	if req.TraceID == "" {
		return nil, fmt.Errorf("traceId is required")
	}
	if req.NoticeID <= 0 {
		return nil, fmt.Errorf("noticeId is required")
	}
	if req.OrderID <= 0 {
		return nil, fmt.Errorf("orderId is required")
	}
	if req.SenderUserID <= 0 || req.ReceiverUserID <= 0 {
		return nil, fmt.Errorf("senderUserId and receiverUserId are required")
	}
	if req.OliveBranchRecordID <= 0 {
		return nil, fmt.Errorf("oliveBranchRecordId is required")
	}
	if strings.TrimSpace(req.Content) == "" {
		return nil, fmt.Errorf("content is required")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/api/v2/sms/send",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("post sms notice: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var envelope smsNoticeEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if envelope.Code != http.StatusOK {
		return nil, fmt.Errorf("message center code %d: %s", envelope.Code, envelope.Message)
	}
	if envelope.Data == nil {
		return &SmsNoticeResponse{}, nil
	}
	return envelope.Data, nil
}

func (c *Client) SendRegisterInviteSms(ctx context.Context, req RegisterInviteSmsRequest) (*RegisterInviteSmsResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("message center client is nil")
	}
	if req.TraceID == "" {
		return nil, fmt.Errorf("traceId is required")
	}
	if req.RecordID <= 0 {
		return nil, fmt.Errorf("recordId is required")
	}
	if strings.TrimSpace(req.Phone) == "" {
		return nil, fmt.Errorf("phone is required")
	}
	if req.ProjectID <= 0 {
		return nil, fmt.Errorf("projectId is required")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/api/v2/sms/register-invite",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("post register invite sms: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var envelope registerInviteSmsEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if envelope.Code != http.StatusOK {
		return nil, fmt.Errorf("message center code %d: %s", envelope.Code, envelope.Message)
	}
	if envelope.Data == nil {
		return &RegisterInviteSmsResponse{}, nil
	}
	return envelope.Data, nil
}

func (c *Client) SendAdminSms(ctx context.Context, req AdminSmsSendRequest) (*AdminSmsSendResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("message center client is nil")
	}
	if strings.TrimSpace(req.TemplateKey) == "" {
		return nil, fmt.Errorf("template_key is required")
	}
	if req.UserID <= 0 {
		return nil, fmt.Errorf("user_id is required")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/api/v2/admin/sms/send",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("post admin sms: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var envelope adminSmsSendEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if envelope.Code != http.StatusOK {
		return nil, fmt.Errorf("message center code %d: %s", envelope.Code, envelope.Message)
	}
	if envelope.Data == nil {
		return nil, fmt.Errorf("message center response data is empty")
	}
	return envelope.Data, nil
}

func (c *Client) CountAdminSms(ctx context.Context, userID int, templateKey string, days int) (*AdminSmsSendCountResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("message center client is nil")
	}
	if userID <= 0 {
		return nil, fmt.Errorf("user_id is required")
	}
	if strings.TrimSpace(templateKey) == "" {
		return nil, fmt.Errorf("template_key is required")
	}
	if days <= 0 {
		return nil, fmt.Errorf("days is required")
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.baseURL+"/api/v2/admin/sms/send-count",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	q := httpReq.URL.Query()
	q.Set("user_id", fmt.Sprintf("%d", userID))
	q.Set("template_key", templateKey)
	q.Set("days", fmt.Sprintf("%d", days))
	httpReq.URL.RawQuery = q.Encode()
	httpReq.Header.Set("Authorization", "Bearer "+c.apiToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("get admin sms count: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var envelope adminSmsSendCountEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if envelope.Code != http.StatusOK {
		return nil, fmt.Errorf("message center code %d: %s", envelope.Code, envelope.Message)
	}
	if envelope.Data == nil {
		return nil, fmt.Errorf("message center response data is empty")
	}
	return envelope.Data, nil
}
