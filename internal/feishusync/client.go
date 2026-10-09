package feishusync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type Target struct {
	SchoolID     int    `db:"school_id"`
	AppToken     string `db:"app_token"`
	NodeToken    string `db:"node_token"`
	TableID      string `db:"table_id"`
	ViewID       string `db:"view_id"`
	NodeStarted  bool   `db:"node_started"`
	TableStarted bool   `db:"table_started"`
}

// Keep batches modest to bound payloads, response size and permission-check intervals.
const updateBatchSize = 50
const updateBatchBytes = 1024 * 1024

type RecordUpdate struct {
	RecordID string         `json:"record_id"`
	Fields   map[string]any `json:"fields"`
}

type Remote interface {
	CreateNode(context.Context, string) (string, string, error)
	CreateTable(context.Context, Target) (string, string, error)
	GetFields(context.Context, Target) ([]Field, error)
	CreateRecord(context.Context, Target, string, map[string]any) (string, error)
	UpdateRecord(context.Context, Target, string, map[string]any) error
	UpdateRecords(context.Context, Target, []RecordUpdate) error
	DeleteRecord(context.Context, Target, string) error
}

type APIError struct{ Code, Status int }

func (e *APIError) Error() string {
	return fmt.Sprintf("飞书接口失败（HTTP %d，错误码 %d），请检查应用权限及资源授权", e.Status, e.Code)
}
func isMissingRecord(err error) bool { e, ok := err.(*APIError); return ok && e.Code == 1254043 }

type Client struct {
	appID, secret, space, parent string
	baseURL                      string
	http                         *http.Client
	mu                           sync.Mutex
	token                        string
	expires                      time.Time
	requestMu                    sync.Mutex
	nextRequest                  time.Time
}

func NewClient(appID, secret, space, parent string) *Client {
	return &Client{appID: appID, secret: secret, space: space, parent: parent, baseURL: "https://open.feishu.cn", http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.expires) {
		return c.token, nil
	}
	var result struct {
		Token  string `json:"tenant_access_token"`
		Expire int    `json:"expire"`
	}
	if err := c.request(ctx, http.MethodPost, "/open-apis/auth/v3/tenant_access_token/internal", "", map[string]string{"app_id": c.appID, "app_secret": c.secret}, &result); err != nil {
		return "", err
	}
	if result.Token == "" || result.Expire <= 60 {
		return "", fmt.Errorf("飞书应用授权响应无效")
	}
	c.token, c.expires = result.Token, time.Now().Add(time.Duration(result.Expire-60)*time.Second)
	return c.token, nil
}

// Do not retry mutations automatically: a timed-out create may already exist remotely.
func (c *Client) request(ctx context.Context, method, path, token string, body, out any) error {
	// One worker owns the shared DB lock; cap application calls at two per second.
	c.requestMu.Lock()
	wait := time.Until(c.nextRequest)
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			c.requestMu.Unlock()
			return fmt.Errorf("同步请求已取消")
		case <-timer.C:
		}
	}
	c.nextRequest = time.Now().Add(500 * time.Millisecond)
	c.requestMu.Unlock()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("飞书请求编码失败")
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("飞书请求构建失败")
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("飞书网络请求未能确认结果，请稍后检查同步状态")
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8*1024*1024))
	if err != nil {
		return fmt.Errorf("飞书响应读取失败，操作结果待核对")
	}
	var envelope struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("飞书响应格式无效，操作结果待核对")
	}
	if envelope.Code == nil {
		return fmt.Errorf("飞书响应缺少结果码，操作结果待核对")
	}
	if *envelope.Code != 0 || res.StatusCode < 200 || res.StatusCode >= 300 {
		return &APIError{Code: *envelope.Code, Status: res.StatusCode}
	}
	if out == nil {
		return nil
	}
	// Auth response has no data envelope.
	if token == "" {
		return json.Unmarshal(raw, out)
	}
	if err = json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("飞书响应内容无效，操作结果待核对")
	}
	return nil
}

func (c *Client) call(ctx context.Context, method, path string, body, out any) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	return c.request(ctx, method, path, token, body, out)
}
func tablePath(t Target) string {
	return "/open-apis/bitable/v1/apps/" + url.PathEscape(t.AppToken) + "/tables/" + url.PathEscape(t.TableID)
}

func (c *Client) CreateNode(ctx context.Context, title string) (string, string, error) {
	var out struct {
		Node struct {
			Token string `json:"node_token"`
			App   string `json:"obj_token"`
		} `json:"node"`
	}
	err := c.call(ctx, http.MethodPost, "/open-apis/wiki/v2/spaces/"+url.PathEscape(c.space)+"/nodes", map[string]any{"obj_type": "bitable", "node_type": "origin", "parent_node_token": c.parent, "title": title}, &out)
	if err != nil {
		return "", "", err
	}
	if out.Node.Token == "" || out.Node.App == "" {
		return "", "", fmt.Errorf("知识库新建结果待核对")
	}
	return out.Node.Token, out.Node.App, nil
}
func (c *Client) CreateTable(ctx context.Context, t Target) (string, string, error) {
	var out struct {
		TableID string `json:"table_id"`
		ViewID  string `json:"default_view_id"`
	}
	err := c.call(ctx, http.MethodPost, "/open-apis/bitable/v1/apps/"+url.PathEscape(t.AppToken)+"/tables", map[string]any{"table": map[string]any{"name": "用户名单", "default_view_name": "用户名单", "fields": Fields()}}, &out)
	if err != nil {
		return "", "", err
	}
	if out.TableID == "" || out.ViewID == "" {
		return "", "", fmt.Errorf("多维表格新建结果待核对")
	}
	return out.TableID, out.ViewID, nil
}
func (c *Client) GetFields(ctx context.Context, t Target) ([]Field, error) {
	var out struct {
		Items   []Field `json:"items"`
		HasMore bool    `json:"has_more"`
	}
	err := c.call(ctx, http.MethodGet, tablePath(t)+"/fields?page_size=100&view_id="+url.QueryEscape(t.ViewID), nil, &out)
	if err != nil {
		return nil, err
	}
	if out.HasMore {
		return nil, fmt.Errorf("飞书列数量超出原 15 列，请恢复表格结构")
	}
	return out.Items, nil
}
func (c *Client) CreateRecord(ctx context.Context, t Target, key string, fields map[string]any) (string, error) {
	var out struct {
		Record struct {
			ID string `json:"record_id"`
		} `json:"record"`
	}
	err := c.call(ctx, http.MethodPost, tablePath(t)+"/records?client_token="+url.QueryEscape(key), map[string]any{"fields": fields}, &out)
	if err != nil {
		return "", err
	}
	if out.Record.ID == "" {
		return "", fmt.Errorf("飞书新增记录结果待核对")
	}
	return out.Record.ID, nil
}
func (c *Client) UpdateRecord(ctx context.Context, t Target, id string, fields map[string]any) error {
	return c.call(ctx, http.MethodPut, tablePath(t)+"/records/"+url.PathEscape(id), map[string]any{"fields": fields}, nil)
}

// Require confirmation of every requested ID; code=0 alone is insufficient.
// Missing or malformed acknowledgements must never trigger replacement creates.
func (c *Client) UpdateRecords(ctx context.Context, t Target, records []RecordUpdate) (result error) {
	if len(records) == 0 || len(records) > updateBatchSize {
		return fmt.Errorf("飞书批量更新条数无效")
	}
	requested := make(map[string]bool, len(records))
	for _, record := range records {
		if record.RecordID == "" || requested[record.RecordID] {
			return fmt.Errorf("飞书批量更新记录 ID 无效或重复")
		}
		requested[record.RecordID] = true
	}
	body := struct {
		Records []RecordUpdate `json:"records"`
	}{records}
	payload, err := json.Marshal(body)
	if err != nil || len(payload) > updateBatchBytes {
		return fmt.Errorf("飞书批量更新请求过大或编码失败")
	}
	var out struct {
		Records []struct {
			ID string `json:"record_id"`
		} `json:"records"`
	}
	started := time.Now()
	defer func() {
		log.Printf("feishu sync batch update records=%d elapsed_ms=%d confirmed=%t", len(records), time.Since(started).Milliseconds(), result == nil)
	}()
	if err := c.call(ctx, http.MethodPost, tablePath(t)+"/records/batch_update", body, &out); err != nil {
		return err
	}
	if len(out.Records) != len(records) {
		return fmt.Errorf("飞书批量更新结果不完整，请重新同步；记录映射已保留")
	}
	for _, record := range out.Records {
		if !requested[record.ID] {
			return fmt.Errorf("飞书批量更新结果无效，请重新同步；记录映射已保留")
		}
		delete(requested, record.ID)
	}
	return nil
}

func (c *Client) DeleteRecord(ctx context.Context, t Target, id string) error {
	return c.call(ctx, http.MethodDelete, tablePath(t)+"/records/"+url.PathEscape(id), nil, nil)
}
