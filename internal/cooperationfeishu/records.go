package cooperationfeishu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type application struct {
	ID         int64      `db:"id"`
	UserID     int        `db:"user_id"`
	AttemptNo  int        `db:"attempt_no"`
	Status     string     `db:"status"`
	Profile    *string    `db:"profile_snapshot"`
	AppliedAt  time.Time  `db:"applied_at"`
	ReviewedAt *time.Time `db:"reviewed_at"`
	Remark     *string    `db:"review_remark"`
}

type record struct {
	ApplicationID int64   `db:"application_id"`
	RecordID      *string `db:"record_id"`
	State         string  `db:"state"`
}

func (s *Service) syncRecords(ctx context.Context, cooperationID int64, appToken, tableID string) error {
	rows := []application{}
	if err := s.db.SelectContext(ctx, &rows, `SELECT id,user_id,attempt_no,status,profile_snapshot,applied_at,reviewed_at,review_remark FROM campus_cooperation_application WHERE cooperation_id=? ORDER BY id`, cooperationID); err != nil {
		return err
	}
	mappings := []record{}
	if err := s.db.SelectContext(ctx, &mappings, `SELECT application_id,record_id,state FROM cooperation_feishu_record WHERE cooperation_id=?`, cooperationID); err != nil {
		return err
	}
	byID := make(map[int64]record, len(mappings))
	for _, mapping := range mappings {
		byID[mapping.ApplicationID] = mapping
	}
	seen := make(map[int64]bool, len(rows))
	path := "/open-apis/bitable/v1/apps/" + url.PathEscape(appToken) + "/tables/" + url.PathEscape(tableID) + "/records"
	for i, row := range rows {
		if i%50 == 0 {
			if _, err := s.cooperation.AdminDetail(ctx, s.currentAdminID(ctx), cooperationID, false); err != nil {
				return err
			}
			if err := s.private(ctx, appToken); err != nil {
				return err
			}
		}
		seen[row.ID] = true
		values, err := row.values()
		if err != nil {
			return err
		}
		mapping, exists := byID[row.ID]
		if !exists {
			key := "cooperation-application-" + strconv.FormatInt(row.ID, 10)
			if _, err = s.db.ExecContext(ctx, `INSERT INTO cooperation_feishu_record(application_id,cooperation_id,client_token,state) VALUES(?,?,?,'pending')`, row.ID, cooperationID, key); err != nil {
				return err
			}
			var out struct {
				Record struct {
					ID string `json:"record_id"`
				} `json:"record"`
			}
			if err = s.client.Call(ctx, http.MethodPost, path+"?client_token="+url.QueryEscape(key), map[string]any{"fields": values}, &out); err != nil {
				return errNeedsAttention
			}
			if out.Record.ID == "" {
				return errNeedsAttention
			}
			if _, err = s.db.ExecContext(ctx, `UPDATE cooperation_feishu_record SET record_id=?,state='ready' WHERE application_id=?`, out.Record.ID, row.ID); err != nil {
				return errNeedsAttention
			}
		} else {
			if mapping.State != "ready" || mapping.RecordID == nil || *mapping.RecordID == "" {
				return errNeedsAttention
			}
			if err = s.client.Call(ctx, http.MethodPut, path+"/"+url.PathEscape(*mapping.RecordID), map[string]any{"fields": values}, nil); err != nil {
				return err
			}
		}
	}
	// A user deletion removes their application locally; remove the remote private row too.
	for _, mapping := range mappings {
		if seen[mapping.ApplicationID] {
			continue
		}
		if mapping.State != "ready" || mapping.RecordID == nil {
			return errNeedsAttention
		}
		if err := s.client.Call(ctx, http.MethodDelete, path+"/"+url.PathEscape(*mapping.RecordID), nil, nil); err != nil {
			return errNeedsAttention
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM cooperation_feishu_record WHERE application_id=?`, mapping.ApplicationID); err != nil {
			return errNeedsAttention
		}
	}
	return nil
}

func (a application) values() (map[string]any, error) {
	card := map[string]any{}
	if a.Profile != nil && *a.Profile != "" {
		if err := json.Unmarshal([]byte(*a.Profile), &card); err != nil {
			return nil, fmt.Errorf("申请名片快照无效")
		}
	}
	get := func(key string) any {
		value := card[key]
		if value == nil {
			return nil
		}
		if items, ok := value.([]any); ok {
			parts := make([]string, 0, len(items))
			for _, item := range items {
				parts = append(parts, fmt.Sprint(item))
			}
			return strings.Join(parts, "、")
		}
		return fmt.Sprint(value)
	}
	status := map[string]string{"pending": "待审核", "approved": "已通过", "rejected": "已拒绝"}[a.Status]
	if status == "" {
		return nil, fmt.Errorf("申请状态无效")
	}
	values := map[string]any{
		"申请ID": strconv.FormatInt(a.ID, 10), "用户ID": strconv.Itoa(a.UserID), "申请轮次": strconv.Itoa(a.AttemptNo),
		"审核状态": status, "申请时间": a.AppliedAt.Format(time.DateTime),
		"昵称": get("nickname"), "学校": get("schoolName"), "专业": get("majorName"), "入学年份": get("grade"),
		"手机": get("phone"), "微信": get("wechat"), "邮箱": get("email"), "技能": get("skills"),
		"MBTI": get("mbti"), "自我评价": get("selfEvaluation"), "项目经历": get("projectExperience"), "协作分": get("collaborationScore"),
		"审核时间": nil, "审核备注": nil,
	}
	if a.ReviewedAt != nil {
		values["审核时间"] = a.ReviewedAt.Format(time.DateTime)
	}
	if a.Remark != nil {
		values["审核备注"] = *a.Remark
	}
	for key, value := range values {
		if text, ok := value.(string); ok && len([]rune(text)) > 100000 {
			return nil, fmt.Errorf("申请 %d 字段 %s 超过飞书长度限制", a.ID, key)
		}
	}
	return values, nil
}

// A single request starts one background run. The admin ID is retained only in
// memory and rechecked while this run is active; it is never written to Feishu.
type adminContextKey struct{}

func withAdminID(ctx context.Context, id int) context.Context {
	return context.WithValue(ctx, adminContextKey{}, id)
}
func (s *Service) currentAdminID(ctx context.Context) int {
	id, _ := ctx.Value(adminContextKey{}).(int)
	return id
}
