package feishusync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"
)

const defaultCleanupWarning = "用户名单同步完成；默认空白项目未全部清理，请管理员检查飞书清理权限及服务日志"

type defaultBlockCleaner interface {
	CleanupDefaultBlocks(context.Context, Target, func() error) (bool, error)
}

// Base v3 uses the same app credentials via the official CLI's client_credentials
// exchange. Keep its cache separate from the existing Wiki/Bitable v1 token.
func (c *Client) baseAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.baseToken != "" && time.Now().Before(c.baseExpires) {
		return c.baseToken, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {c.appID}, "client_secret": {c.secret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.accountsURL+"/oauth/v3/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("飞书清理授权请求构建失败")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := c.waitRequest(ctx); err != nil {
		return "", err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("飞书清理授权请求失败")
	}
	defer res.Body.Close()
	var out struct {
		Code   *int   `json:"code"`
		Token  string `json:"access_token"`
		Expire int    `json:"expires_in"`
		Type   string `json:"token_type"`
	}
	err = json.NewDecoder(io.LimitReader(res.Body, 1024*1024)).Decode(&out)
	if err != nil || res.StatusCode != http.StatusOK || out.Code == nil || *out.Code != 0 || out.Token == "" || out.Expire <= 60 || !strings.EqualFold(out.Type, "Bearer") {
		return "", fmt.Errorf("飞书清理应用授权失败，请检查应用权限")
	}
	c.baseToken, c.baseExpires = out.Token, time.Now().Add(time.Duration(out.Expire-60)*time.Second)
	return c.baseToken, nil
}

func (c *Client) baseCall(ctx context.Context, method, path string, body, out any) error {
	token, err := c.baseAccessToken(ctx)
	if err != nil {
		return err
	}
	return c.request(ctx, method, path, token, body, out)
}
func basePath(t Target) string { return "/open-apis/base/v3/bases/" + url.PathEscape(t.AppToken) }

type baseBlock struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	Parent *string `json:"parent_id"`
	Count  *int    `json:"records_count"`
	Rev    *int    `json:"rev"`
}

func (c *Client) defaultBlocks(ctx context.Context, t Target) ([]baseBlock, error) {
	var out struct {
		Blocks  *[]baseBlock `json:"blocks"`
		Total   *int         `json:"total"`
		HasMore bool         `json:"has_more"`
	}
	if err := c.baseCall(ctx, http.MethodPost, basePath(t)+"/blocks/list", map[string]any{}, &out); err != nil {
		return nil, err
	}
	if out.Blocks == nil || out.Total == nil || out.HasMore || *out.Total != len(*out.Blocks) || *out.Total > 100 {
		return nil, fmt.Errorf("飞书项目目录未完整确认")
	}
	seen := map[string]bool{}
	business := false
	for _, b := range *out.Blocks {
		if b.ID == "" || seen[b.ID] {
			return nil, fmt.Errorf("飞书项目目录无效")
		}
		seen[b.ID] = true
		if b.ID == t.TableID && b.Type == "table" {
			business = true
		}
	}
	if !business {
		return nil, fmt.Errorf("飞书业务数据表未确认，保留默认项目")
	}
	return *out.Blocks, nil
}
func defaultCandidate(b baseBlock, t Target) bool {
	if b.ID == t.TableID || (b.Parent != nil && *b.Parent != "") {
		return false
	}
	switch b.Type {
	case "table":
		return b.Name == "Table" && b.Rev != nil && *b.Rev == 0 && b.Count != nil && (*b.Count == 0 || *b.Count == 10)
	case "dashboard":
		return b.Name == "Dashboard"
	case "workflow":
		return b.Name == "Workflow"
	}
	return false
}

// Only verified default templates in this school's mapped Base are eligible.
// Metadata/auth/network failures retain resources and do not fail user sync.
// Guard failures still stop the task; no mutation follows a lost permission/lock.
func (c *Client) CleanupDefaultBlocks(root context.Context, t Target, guard func() error) (warning bool, result error) {
	if t.AppToken == "" || t.TableID == "" {
		return true, nil
	}
	ctx, cancel := context.WithTimeout(root, 45*time.Second)
	defer cancel()
	warn := func(err error) {
		warning = true
		// Sanitized client errors contain no remote payload, tokens or personal data.
		log.Printf("feishu default cleanup school=%d warning=%s", t.SchoolID, safeError(err))
	}
	if err := guard(); err != nil {
		return false, err
	}
	blocks, err := c.defaultBlocks(ctx, t)
	if err != nil {
		warn(err)
		return warning, nil
	}
	names := map[string]int{}
	for _, b := range blocks {
		names[b.Type+":"+b.Name]++
	}
	for _, b := range blocks {
		if !defaultCandidate(b, t) || names[b.Type+":"+b.Name] != 1 {
			continue
		}
		if err := guard(); err != nil {
			return warning, err
		}
		empty, err := c.isDefaultBlock(ctx, t, b)
		if err != nil {
			warn(err)
			continue
		}
		if !empty {
			continue
		}
		// Refresh directory identity/revision after reading contents. The remote API
		// has no atomic empty-only delete; administrators must not edit defaults
		// during sync. Never delete the mapped business table or renamed projects.
		fresh, err := c.defaultBlocks(ctx, t)
		if err != nil {
			warn(err)
			continue
		}
		unchanged := false
		matches := 0
		for _, current := range fresh {
			if current.Type == b.Type && current.Name == b.Name {
				matches++
			}
			if reflect.DeepEqual(b, current) {
				unchanged = true
			}
		}
		if !unchanged || matches != 1 {
			continue
		}
		if err := guard(); err != nil {
			return warning, err
		}
		if err := c.baseCall(ctx, http.MethodDelete, basePath(t)+"/blocks/"+url.PathEscape(b.ID), nil, nil); err != nil {
			warn(err)
			continue
		}
		log.Printf("feishu default cleanup school=%d type=%s deleted=true", t.SchoolID, b.Type)
	}
	return warning, nil
}

func (c *Client) isDefaultBlock(ctx context.Context, t Target, b baseBlock) (bool, error) {
	switch b.Type {
	case "table":
		return c.isDefaultTable(ctx, t, b)
	case "dashboard":
		var out struct {
			ID     string             `json:"dashboard_id"`
			Name   string             `json:"name"`
			Blocks *[]json.RawMessage `json:"blocks"`
			Theme  map[string]any     `json:"theme"`
		}
		err := c.baseCall(ctx, http.MethodGet, basePath(t)+"/dashboards/"+url.PathEscape(b.ID), nil, &out)
		return out.ID == b.ID && out.Name == "Dashboard" && out.Blocks != nil && len(*out.Blocks) == 0 && reflect.DeepEqual(out.Theme, map[string]any{"theme_style": "default"}), err
	case "workflow":
		var out struct {
			ID      string             `json:"workflow_id"`
			Title   string             `json:"title"`
			Status  string             `json:"status"`
			Steps   *[]json.RawMessage `json:"steps"`
			Created *int64             `json:"create_time"`
			Updated *int64             `json:"update_time"`
			Creator string             `json:"creator_id"`
			Updater string             `json:"updater_id"`
		}
		err := c.baseCall(ctx, http.MethodGet, basePath(t)+"/workflows/"+url.PathEscape(b.ID), nil, &out)
		return out.ID == b.ID && out.Title == "Workflow" && out.Status == "disabled" && out.Steps != nil && len(*out.Steps) == 0 && out.Created != nil && out.Updated != nil && *out.Created == *out.Updated && out.Creator != "" && out.Creator == out.Updater, err
	}
	return false, nil
}

func (c *Client) isDefaultTable(ctx context.Context, t Target, b baseBlock) (bool, error) {
	path := basePath(t) + "/tables/" + url.PathEscape(b.ID)
	var schema struct {
		Fields  []map[string]any `json:"fields"`
		Total   *int             `json:"total"`
		HasMore bool             `json:"has_more"`
	}
	if err := c.baseCall(ctx, http.MethodGet, path+"/fields?offset=0&limit=5", nil, &schema); err != nil {
		return false, err
	}
	if schema.Total == nil || *schema.Total != 4 || len(schema.Fields) != 4 || schema.HasMore {
		return false, nil
	}
	templates := []map[string]any{
		{"name": "Text", "type": "text", "default_value": nil, "style": map[string]any{"type": "plain"}},
		{"name": "Single option", "type": "select", "multiple": false, "options": []any{}, "default_value": nil},
		{"name": "Date", "type": "datetime", "style": map[string]any{"format": "yyyy/MM/dd"}, "default_value": nil},
		{"name": "Attachment", "type": "attachment", "style": map[string]any{"type": "plain"}},
	}
	query := url.Values{"offset": {"0"}, "limit": {"11"}}
	ids := make([]string, 4)
	seenFields := map[string]bool{}
	for i, field := range schema.Fields {
		id, ok := field["id"].(string)
		if !ok || id == "" || seenFields[id] {
			return false, nil
		}
		seenFields[id] = true
		ids[i] = id
		delete(field, "id")
		if !reflect.DeepEqual(field, templates[i]) {
			return false, nil
		}
		query.Add("field_id", id)
	}
	var views struct {
		Views   []map[string]any `json:"views"`
		Total   *int             `json:"total"`
		HasMore bool             `json:"has_more"`
	}
	if err := c.baseCall(ctx, http.MethodGet, path+"/views?offset=0&limit=2", nil, &views); err != nil {
		return false, err
	}
	if views.Total == nil || *views.Total != 1 || len(views.Views) != 1 || views.HasMore {
		return false, nil
	}
	view := views.Views[0]
	viewID, ok := view["id"].(string)
	if !ok || viewID == "" {
		return false, nil
	}
	delete(view, "id")
	if !reflect.DeepEqual(view, map[string]any{"name": "Grid", "type": "grid", "_meta": map[string]any{"filter": nil, "group": []any{}, "sort": []any{}, "visible_fields": "4 fields"}}) {
		return false, nil
	}
	var records struct {
		IDs     []string `json:"record_id_list"`
		Fields  []string `json:"field_id_list"`
		Data    *[][]any `json:"data"`
		HasMore *bool    `json:"has_more"`
		Rev     *int     `json:"rev"`
		Scope   struct {
			Record string `json:"record_scope"`
		} `json:"query_context"`
	}
	if err := c.baseCall(ctx, http.MethodGet, path+"/records?"+query.Encode(), nil, &records); err != nil {
		return false, err
	}
	if records.Data == nil || records.HasMore == nil || *records.HasMore || records.Rev == nil || *records.Rev != 0 || !reflect.DeepEqual(records.Fields, ids) || records.Scope.Record != "all_records" {
		return false, nil
	}
	if len(*records.Data) != *b.Count || len(records.IDs) != len(*records.Data) {
		return false, nil
	}
	seen := map[string]bool{}
	for i, row := range *records.Data {
		if records.IDs[i] == "" || seen[records.IDs[i]] || len(row) != 4 {
			return false, nil
		}
		seen[records.IDs[i]] = true
		for _, value := range row {
			if value != nil {
				return false, nil
			}
		}
	}
	return true, nil
}
