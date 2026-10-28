package cooperationfeishu

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/feishusync"
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
	Talent     int        `db:"talent_status"`
	UserStatus int        `db:"user_status"`
}

type record struct {
	ApplicationID int64   `db:"application_id"`
	RecordID      *string `db:"record_id"`
	ClientToken   string  `db:"client_token"`
	State         string  `db:"state"`
}

func (s *Service) syncRecords(ctx context.Context, cooperationID int64, appToken, tableID string) error {
	var maxID int64
	if err := s.db.GetContext(ctx, &maxID, `SELECT COALESCE(MAX(id),0) FROM campus_cooperation_application WHERE cooperation_id=?`, cooperationID); err != nil {
		return err
	}
	path := "/open-apis/bitable/v1/apps/" + url.PathEscape(appToken) + "/tables/" + url.PathEscape(tableID) + "/records"
	batch := make([]feishusync.RecordUpdate, 0, 50)
	batchBytes := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := s.cleanup.UpdateRecords(ctx, feishusync.Target{AppToken: appToken, TableID: tableID}, batch); err != nil {
			return err
		}
		batch = batch[:0]
		batchBytes = 0
		return nil
	}
	var lastUserID, lastAttemptNo int
	for {
		rows := []application{}
		// This order uses the existing unique (cooperation_id,user_id,attempt_no)
		// index; paging by id would repeatedly sort all applications for a school.
		if err := s.db.SelectContext(ctx, &rows, "SELECT a.id,a.user_id,a.attempt_no,a.status,a.profile_snapshot,a.applied_at,a.reviewed_at,a.review_remark,COALESCE(tp.status,0) AS talent_status,COALESCE(u.user_status,0) AS user_status FROM campus_cooperation_application a LEFT JOIN talent_profile tp ON tp.user_id=a.user_id LEFT JOIN `user` u ON u.id=a.user_id WHERE a.cooperation_id=? AND a.id<=? AND (a.user_id>? OR (a.user_id=? AND a.attempt_no>?)) ORDER BY a.user_id,a.attempt_no LIMIT 50", cooperationID, maxID, lastUserID, lastUserID, lastAttemptNo); err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		if _, err := s.cooperation.AdminDetail(ctx, s.currentAdminID(ctx), cooperationID, false); err != nil {
			return err
		}
		ids := make([]int64, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		query, args, err := sqlx.In(`SELECT application_id,record_id,client_token,state FROM cooperation_feishu_record WHERE cooperation_id=? AND application_id IN (?)`, cooperationID, ids)
		if err != nil {
			return err
		}
		mappings := []record{}
		if err = s.db.SelectContext(ctx, &mappings, s.db.Rebind(query), args...); err != nil {
			return err
		}
		byID := make(map[int64]record, len(mappings))
		for _, mapping := range mappings {
			byID[mapping.ApplicationID] = mapping
		}
		for _, row := range rows {
			lastUserID, lastAttemptNo = row.UserID, row.AttemptNo
			mapping, exists := byID[row.ID]
			values, err := row.values()
			if err != nil {
				return err
			}
			if !exists {
				key := uuid.NewString()
				if _, err = s.db.ExecContext(ctx, `INSERT INTO cooperation_feishu_record(application_id,cooperation_id,client_token,state) VALUES(?,?,?,'pending')`, row.ID, cooperationID, key); err != nil {
					return err
				}
				mapping = record{ApplicationID: row.ID, ClientToken: key, State: "pending"}
				exists = true
			}
			if mapping.State == "pending" && mapping.RecordID == nil {
				if err := flush(); err != nil {
					return err
				}
				key := mapping.ClientToken
				if !isUUIDv4(key) {
					// Earlier versions used a non-UUID token that Feishu rejects before
					// creating a record. Replace only that known legacy token format.
					if !strings.HasPrefix(key, "cooperation-application-") {
						return errNeedsAttention
					}
					key = uuid.NewString()
					result, updateErr := s.db.ExecContext(ctx, `UPDATE cooperation_feishu_record SET client_token=? WHERE application_id=? AND cooperation_id=? AND record_id IS NULL AND state='pending' AND client_token=?`, key, row.ID, cooperationID, mapping.ClientToken)
					if updateErr != nil {
						return errNeedsAttention
					}
					changed, rowsErr := result.RowsAffected()
					if rowsErr != nil || changed != 1 {
						return errNeedsAttention
					}
				}
				var out struct {
					Record struct {
						ID string `json:"record_id"`
					} `json:"record"`
				}
				if err = s.client.Call(ctx, http.MethodPost, path+"?client_token="+url.QueryEscape(key), map[string]any{"fields": values}, &out); err != nil {
					return fmt.Errorf("%w: %w", errNeedsAttention, err)
				}
				if out.Record.ID == "" {
					return errNeedsAttention
				}
				result, updateErr := s.db.ExecContext(ctx, `UPDATE cooperation_feishu_record SET record_id=?,state='ready' WHERE application_id=? AND cooperation_id=? AND state='pending' AND record_id IS NULL`, out.Record.ID, row.ID, cooperationID)
				if updateErr != nil {
					return errNeedsAttention
				}
				changed, rowsErr := result.RowsAffected()
				if rowsErr != nil || changed != 1 {
					return errNeedsAttention
				}
			} else if mapping.State == "ready" && mapping.RecordID != nil && *mapping.RecordID != "" {
				update := feishusync.RecordUpdate{RecordID: *mapping.RecordID, Fields: values}
				encoded, encodeErr := json.Marshal(update)
				if encodeErr != nil {
					return encodeErr
				}
				// Leave headroom below the one-megabyte Feishu batch limit.
				if len(batch) == 50 || batchBytes+len(encoded)+1 > 900*1024 {
					if err := flush(); err != nil {
						return err
					}
				}
				if len(encoded)+32 > 900*1024 {
					if err := s.client.Call(ctx, http.MethodPut, path+"/"+url.PathEscape(*mapping.RecordID), map[string]any{"fields": values}, nil); err != nil {
						return err
					}
				} else {
					batch = append(batch, update)
					batchBytes += len(encoded) + 1
				}
			} else {
				return errNeedsAttention
			}
		}
		if err := flush(); err != nil {
			return err
		}
	}
	// A user deletion removes their application locally; remove the remote private row too.
	for {
		stale := []record{}
		if err := s.db.SelectContext(ctx, &stale, `SELECT r.application_id,r.record_id,r.client_token,r.state FROM cooperation_feishu_record r LEFT JOIN campus_cooperation_application a ON a.id=r.application_id WHERE r.cooperation_id=? AND a.id IS NULL LIMIT 50`, cooperationID); err != nil {
			return err
		}
		if len(stale) == 0 {
			break
		}
		if _, err := s.cooperation.AdminDetail(ctx, s.currentAdminID(ctx), cooperationID, false); err != nil {
			return err
		}
		for _, mapping := range stale {
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
	}
	return nil
}

func isUUIDv4(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.Version() == uuid.Version(4)
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
	textValue := func(key string) string {
		value := get(key)
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
	status := map[string]string{"pending": "未处理", "approved": "成功", "rejected": "失败"}[a.Status]
	if status == "" {
		return nil, fmt.Errorf("申请状态无效")
	}
	floatValue := func(key string) float64 {
		switch value := card[key].(type) {
		case float64:
			return value
		case json.Number:
			result, _ := value.Float64()
			return result
		case string:
			result, _ := strconv.ParseFloat(value, 64)
			return result
		default:
			return 0
		}
	}
	score, scoreExists := card["collaborationScore"]
	row := feishusync.UserRow{
		ID: a.UserID, Nickname: textValue("nickname"), MBTI: textValue("mbti"),
		School: textValue("schoolName"), Major: textValue("majorName"), Grade: textValue("grade"),
		Intro: textValue("selfEvaluation"), Experience: textValue("projectExperience"),
		Score: sql.NullFloat64{Float64: floatValue("collaborationScore"), Valid: scoreExists && score != nil},
		Auth:  int(floatValue("authStatus")), Talent: a.Talent, Phone: textValue("phone"),
		Wechat: textValue("wechat"), Email: textValue("email"), Status: a.UserStatus,
	}
	values, err := row.Values()
	if err != nil {
		return nil, err
	}
	values["申请结果"] = status
	values["申请轮次"] = a.AttemptNo
	values["申请时间"] = a.AppliedAt.Format(time.DateTime)
	values["审核时间"] = nil
	values["审核备注"] = nil
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
