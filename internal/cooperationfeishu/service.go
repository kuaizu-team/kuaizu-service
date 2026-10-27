package cooperationfeishu

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/feishusync"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
)

const defaultSpaceID = "7701281082951437603"

type Target struct {
	CooperationID int64      `db:"cooperation_id" json:"cooperationId"`
	AppToken      *string    `db:"app_token" json:"-"`
	TableID       *string    `db:"table_id" json:"-"`
	ViewID        *string    `db:"view_id" json:"-"`
	DocBlockID    *string    `db:"doc_block_id" json:"-"`
	Status        string     `db:"status" json:"status"`
	Message       string     `db:"message" json:"message"`
	SyncedAt      *time.Time `db:"synced_at" json:"syncedAt"`
	UpdatedAt     time.Time  `db:"updated_at" json:"updatedAt"`
	WikiURL       string     `db:"-" json:"wikiUrl,omitempty"`
	BaseURL       string     `db:"-" json:"baseUrl,omitempty"`
}

type Service struct {
	db          *sqlx.DB
	cooperation *service.CampusCooperationService
	client      remote
	cleanup     *feishusync.Client
	spaceID     string
	baseURL     string
}

type remote interface {
	Call(context.Context, string, string, any, any) error
}

func NewFromEnv(ctx context.Context, repo *repository.Repository, cooperation *service.CampusCooperationService) (*Service, error) {
	if os.Getenv("FEISHU_COOPERATION_SYNC_ENABLED") != "true" {
		return nil, nil
	}
	app, secret := strings.TrimSpace(os.Getenv("FEISHU_APP_ID")), strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET"))
	if app == "" || secret == "" {
		return nil, fmt.Errorf("合作申请飞书同步缺少应用凭证")
	}
	space := strings.TrimSpace(os.Getenv("FEISHU_COOPERATION_WIKI_SPACE_ID"))
	if space == "" {
		space = defaultSpaceID
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("FEISHU_WIKI_BASE_URL")), "/")
	if base == "" {
		base = "https://rw0q02v5pok.feishu.cn"
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.Path != "" || u.User != nil || (u.Host != "feishu.cn" && !strings.HasSuffix(u.Host, ".feishu.cn")) {
		return nil, fmt.Errorf("飞书知识库域名配置无效")
	}
	client := feishusync.NewClient(app, secret, space, "")
	s := &Service{db: repo.DB(), cooperation: cooperation, client: client, cleanup: client, spaceID: space, baseURL: base}
	for _, query := range []string{
		`SELECT feishu_wiki_node_token FROM commercial_order LIMIT 0`,
		`SELECT cooperation_id,app_token,table_id,view_id,doc_block_id,status,message,synced_at,updated_at FROM cooperation_feishu_target LIMIT 0`,
		`SELECT application_id,cooperation_id,record_id,client_token,state FROM cooperation_feishu_record LIMIT 0`,
	} {
		rows, err := s.db.QueryContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("合作申请飞书同步表未就绪，请先执行 sql/20261027_cooperation_feishu_sync.sql")
		}
		rows.Close()
	}
	return s, nil
}

const targetColumns = `cooperation_id,app_token,table_id,view_id,doc_block_id,status,message,synced_at,updated_at`

func (s *Service) Get(ctx context.Context, adminID int, cooperationID int64) (*Target, error) {
	c, err := s.cooperation.AdminDetail(ctx, adminID, cooperationID, false)
	if err != nil {
		return nil, err
	}
	var t Target
	err = s.db.GetContext(ctx, &t, `SELECT `+targetColumns+` FROM cooperation_feishu_target WHERE cooperation_id=?`, cooperationID)
	if errors.Is(err, sql.ErrNoRows) {
		t = Target{CooperationID: cooperationID, Status: "idle"}
	} else if err != nil {
		return nil, err
	}
	if t.Status == "running" && time.Since(t.UpdatedAt) > 21*time.Minute {
		t.Status, t.Message = "needs_attention", "同步进程可能已中断，请技术人员核对远端表格和记录映射"
	}
	if c.FeishuWikiNodeToken != nil && *c.FeishuWikiNodeToken != "" {
		t.WikiURL = s.baseURL + "/wiki/" + url.PathEscape(*c.FeishuWikiNodeToken)
	}
	if t.AppToken != nil && *t.AppToken != "" {
		t.BaseURL = s.baseURL + "/base/" + url.PathEscape(*t.AppToken)
		if t.DocBlockID != nil && strings.HasPrefix(*t.DocBlockID, "wiki:") {
			t.BaseURL = s.baseURL + "/wiki/" + url.PathEscape(strings.TrimPrefix(*t.DocBlockID, "wiki:"))
		}
	}
	return &t, nil
}

func (s *Service) Start(ctx context.Context, adminID int, cooperationID int64) (*Target, error) {
	c, err := s.cooperation.AdminDetail(ctx, adminID, cooperationID, false)
	if err != nil {
		return nil, err
	}
	if c.FeishuWikiNodeToken == nil || *c.FeishuWikiNodeToken == "" {
		return nil, service.ErrBadRequest("平台超管尚未绑定商单飞书文档")
	}
	if _, err = s.db.ExecContext(ctx, `INSERT IGNORE INTO cooperation_feishu_target(cooperation_id) VALUES(?)`, cooperationID); err != nil {
		return nil, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE cooperation_feishu_target SET status='running',message='正在同步' WHERE cooperation_id=? AND status IN ('idle','succeeded','failed')`, cooperationID)
	if err != nil {
		return nil, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if changed == 1 {
		go s.run(cooperationID, adminID)
	}
	return s.Get(ctx, adminID, cooperationID)
}

func (s *Service) run(cooperationID int64, adminID int) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	ctx = withAdminID(ctx, adminID)
	err := s.sync(ctx, cooperationID, adminID)
	status, message := "succeeded", "申请已同步到商单文档下的学校表格"
	if err != nil {
		status, message = "failed", "同步失败，请检查应用授权和表格权限"
	}
	if errors.Is(err, errNeedsAttention) {
		status, message = "needs_attention", "远端创建结果待核对，请联系技术人员确认后重试"
	}
	if _, dbErr := s.db.ExecContext(context.Background(), `UPDATE cooperation_feishu_target SET status=?,message=?,synced_at=CASE WHEN ?='succeeded' THEN NOW() ELSE synced_at END WHERE cooperation_id=?`, status, message, status, cooperationID); dbErr != nil {
		fmt.Fprintf(os.Stderr, "cooperation feishu status persistence failed cooperation=%d: %v\n", cooperationID, dbErr)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "cooperation feishu sync failed cooperation=%d error_type=%T\n", cooperationID, err)
	}
}

var errNeedsAttention = errors.New("remote operation result needs reconciliation")

func (s *Service) sync(ctx context.Context, cooperationID int64, adminID int) error {
	c, err := s.cooperation.AdminDetail(ctx, adminID, cooperationID, false)
	if err != nil {
		return err
	}
	if c.FeishuWikiNodeToken == nil || *c.FeishuWikiNodeToken == "" {
		return fmt.Errorf("unbound wiki document")
	}
	if _, err := s.document(ctx, *c.FeishuWikiNodeToken); err != nil {
		return err
	}
	var t Target
	if err = s.db.GetContext(ctx, &t, `SELECT `+targetColumns+` FROM cooperation_feishu_target WHERE cooperation_id=?`, cooperationID); err != nil {
		return err
	}
	needsWikiBase := t.AppToken == nil || t.DocBlockID == nil || !strings.HasPrefix(*t.DocBlockID, "wiki:")
	if !needsWikiBase && t.TableID != nil {
		needsWikiBase, err = s.tableSchemaMismatch(ctx, *t.AppToken, *t.TableID)
		if err != nil {
			return err
		}
	}
	if needsWikiBase {
		title := fmt.Sprintf("%s · %s · 合作申请", c.CompanyName, c.SchoolName)
		nodeToken, appToken, createErr := s.createWikiBase(ctx, *c.FeishuWikiNodeToken, title)
		if createErr != nil {
			return errNeedsAttention
		}
		if err = s.replaceTargetBase(cooperationID, nodeToken, appToken); err != nil {
			return errNeedsAttention
		}
		t.AppToken, t.DocBlockID = &appToken, stringPointer("wiki:"+nodeToken)
		t.TableID, t.ViewID = nil, nil
	}
	if t.TableID == nil {
		var out struct {
			TableID string `json:"table_id"`
			ViewID  string `json:"default_view_id"`
		}
		path := "/open-apis/bitable/v1/apps/" + url.PathEscape(*t.AppToken) + "/tables"
		body := map[string]any{"table": map[string]any{"name": "合作申请", "default_view_name": "合作申请", "fields": fields()}}
		if err = s.client.Call(ctx, http.MethodPost, path, body, &out); err != nil {
			return errNeedsAttention
		}
		if out.TableID == "" {
			return errNeedsAttention
		}
		if _, err = s.db.ExecContext(ctx, `UPDATE cooperation_feishu_target SET table_id=?,view_id=? WHERE cooperation_id=?`, out.TableID, out.ViewID, cooperationID); err != nil {
			return errNeedsAttention
		}
		t.TableID, t.ViewID = &out.TableID, &out.ViewID
	}
	if err = s.syncRecords(ctx, cooperationID, *t.AppToken, *t.TableID); err != nil {
		return err
	}
	_, err = s.cleanup.CleanupDefaultBlocks(ctx, feishusync.Target{AppToken: *t.AppToken, TableID: *t.TableID}, func() error {
		_, err := s.cooperation.AdminDetail(ctx, adminID, cooperationID, false)
		return err
	})
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) createWikiBase(ctx context.Context, parentNode, title string) (string, string, error) {
	var out struct {
		Node struct {
			NodeToken string `json:"node_token"`
			AppToken  string `json:"obj_token"`
		} `json:"node"`
	}
	path := "/open-apis/wiki/v2/spaces/" + url.PathEscape(s.spaceID) + "/nodes"
	err := s.client.Call(ctx, http.MethodPost, path, map[string]any{
		"obj_type": "bitable", "node_type": "origin", "parent_node_token": parentNode, "title": title,
	}, &out)
	if err != nil {
		return "", "", err
	}
	if out.Node.NodeToken == "" || out.Node.AppToken == "" {
		return "", "", errNeedsAttention
	}
	return out.Node.NodeToken, out.Node.AppToken, nil
}

func (s *Service) replaceTargetBase(cooperationID int64, nodeToken, appToken string) error {
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM cooperation_feishu_record WHERE cooperation_id=?`, cooperationID); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE cooperation_feishu_target SET app_token=?,table_id=NULL,view_id=NULL,doc_block_id=? WHERE cooperation_id=?`, appToken, "wiki:"+nodeToken, cooperationID); err != nil {
		return err
	}
	return tx.Commit()
}

func stringPointer(value string) *string { return &value }

func (s *Service) tableSchemaMismatch(ctx context.Context, appToken, tableID string) (bool, error) {
	var out struct {
		Items []struct {
			Name     string `json:"field_name"`
			Type     int    `json:"type"`
			Property struct {
				Options []struct {
					Name string `json:"name"`
				} `json:"options"`
				Formatter string `json:"formatter"`
			} `json:"property"`
		} `json:"items"`
		HasMore bool `json:"has_more"`
	}
	path := "/open-apis/bitable/v1/apps/" + url.PathEscape(appToken) + "/tables/" + url.PathEscape(tableID) + "/fields?page_size=100"
	if err := s.client.Call(ctx, http.MethodGet, path, nil, &out); err != nil {
		return false, err
	}
	expected := fields()
	if out.HasMore || len(out.Items) != len(expected) {
		return true, nil
	}
	for index, field := range expected {
		value := field.(feishusync.Field)
		actual := out.Items[index]
		if actual.Name != value.Name || actual.Type != value.Type {
			return true, nil
		}
		if value.Property == nil {
			continue
		}
		if value.Property.Formatter != "" && actual.Property.Formatter != value.Property.Formatter {
			return true, nil
		}
		for _, option := range value.Property.Options {
			found := false
			for _, actualOption := range actual.Property.Options {
				if actualOption.Name == option.Name {
					found = true
					break
				}
			}
			if !found {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *Service) document(ctx context.Context, nodeToken string) (string, error) {
	var out struct {
		Node struct {
			SpaceID string `json:"space_id"`
			Type    string `json:"obj_type"`
			Token   string `json:"obj_token"`
		} `json:"node"`
	}
	err := s.client.Call(ctx, http.MethodGet, "/open-apis/wiki/v2/spaces/get_node?token="+url.QueryEscape(nodeToken), nil, &out)
	if err != nil {
		return "", err
	}
	if out.Node.SpaceID != s.spaceID || out.Node.Type != "docx" || out.Node.Token == "" {
		return "", fmt.Errorf("商单绑定的文档不属于指定知识库或不是飞书文档")
	}
	return out.Node.Token, nil
}

func fields() []any {
	out := make([]any, 0, len(feishusync.Fields())+5)
	for _, field := range feishusync.Fields() {
		out = append(out, field)
	}
	selectField := func(name string, options ...string) feishusync.Field {
		field := feishusync.Field{Name: name, Type: 3, Property: &feishusync.Property{}}
		for _, option := range options {
			field.Property.Options = append(field.Property.Options, feishusync.Option{Name: option})
		}
		return field
	}
	out = append(out,
		selectField("申请结果", "未处理", "成功", "失败"),
		feishusync.Field{Name: "申请轮次", Type: 2},
		feishusync.Field{Name: "申请时间", Type: 1},
		feishusync.Field{Name: "审核时间", Type: 1},
		feishusync.Field{Name: "审核备注", Type: 1},
	)
	return out
}
