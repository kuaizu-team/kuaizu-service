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
