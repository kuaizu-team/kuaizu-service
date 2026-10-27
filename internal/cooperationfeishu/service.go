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
	spaceID     string
	baseURL     string
	ownerOpenID string
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
	owner := strings.TrimSpace(os.Getenv("FEISHU_COOPERATION_OWNER_OPEN_ID"))
	if !strings.HasPrefix(owner, "ou_") {
		return nil, fmt.Errorf("合作申请飞书同步缺少平台飞书管理人 Open ID")
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
	s := &Service{db: repo.DB(), cooperation: cooperation, client: feishusync.NewClient(app, secret, "", ""), spaceID: space, baseURL: base, ownerOpenID: owner}
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
	status, message := "succeeded", "申请已同步到该校独立表格"
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
	docToken, err := s.document(ctx, *c.FeishuWikiNodeToken)
	if err != nil {
		return err
	}
	var t Target
	if err = s.db.GetContext(ctx, &t, `SELECT `+targetColumns+` FROM cooperation_feishu_target WHERE cooperation_id=?`, cooperationID); err != nil {
		return err
	}
	if t.AppToken == nil {
		var out struct {
			App struct {
				Token string `json:"app_token"`
			} `json:"app"`
		}
		name := fmt.Sprintf("%s · %s · 合作申请", c.CompanyName, c.SchoolName)
		if err = s.client.Call(ctx, http.MethodPost, "/open-apis/bitable/v1/apps", map[string]any{"name": name}, &out); err != nil {
			return errNeedsAttention
		}
		if out.App.Token == "" {
			return errNeedsAttention
		}
		if _, err = s.db.ExecContext(ctx, `UPDATE cooperation_feishu_target SET app_token=? WHERE cooperation_id=?`, out.App.Token, cooperationID); err != nil {
			return errNeedsAttention
		}
		t.AppToken = &out.App.Token
	}
	if err = s.private(ctx, *t.AppToken); err != nil {
		return err
	}
	if err = s.ensureOwner(ctx, *t.AppToken); err != nil {
		return err
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
	if t.DocBlockID == nil {
		link := s.baseURL + "/base/" + url.PathEscape(*t.AppToken)
		if err = s.appendLink(ctx, docToken, c.SchoolName+"合作申请表", link, cooperationID); err != nil {
			return errNeedsAttention
		}
	}
	return nil
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

func (s *Service) private(ctx context.Context, appToken string) error {
	path := "/open-apis/drive/v1/permissions/" + url.PathEscape(appToken) + "/public?type=bitable"
	var out any
	if err := s.client.Call(ctx, http.MethodPatch, path, map[string]any{"link_share_entity": "closed", "external_access": false, "share_entity": "only_full_access"}, &out); err != nil {
		return err
	}
	var settings struct {
		Permission struct {
			Link     string `json:"link_share_entity"`
			External string `json:"external_access_entity"`
		} `json:"permission_public"`
	}
	if err := s.client.Call(ctx, http.MethodGet, path, nil, &settings); err != nil {
		return err
	}
	if settings.Permission.Link != "closed" || settings.Permission.External != "closed" {
		return fmt.Errorf("学校申请表未处于受限访问状态")
	}
	return nil
}

func (s *Service) ensureOwner(ctx context.Context, appToken string) error {
	path := "/open-apis/drive/v1/permissions/" + url.PathEscape(appToken) + "/members?type=bitable"
	var members struct {
		Items []struct {
			ID   string `json:"member_id"`
			Type string `json:"member_type"`
			Perm string `json:"perm"`
		} `json:"items"`
	}
	read := func() error { return s.client.Call(ctx, http.MethodGet, path, nil, &members) }
	if err := read(); err != nil {
		return err
	}
	for _, member := range members.Items {
		if member.ID == s.ownerOpenID && member.Type == "openid" {
			if member.Perm != "full_access" {
				return fmt.Errorf("飞书管理人缺少申请表管理权限")
			}
			return nil
		}
	}
	var out any
	if err := s.client.Call(ctx, http.MethodPost, path, map[string]any{"member_id": s.ownerOpenID, "member_type": "openid", "perm": "full_access"}, &out); err != nil {
		return err
	}
	members.Items = nil
	if err := read(); err != nil {
		return err
	}
	for _, member := range members.Items {
		if member.ID == s.ownerOpenID && member.Type == "openid" && member.Perm == "full_access" {
			return nil
		}
	}
	return fmt.Errorf("飞书管理人权限未确认")
}

func (s *Service) appendLink(ctx context.Context, docToken, title, link string, cooperationID int64) error {
	path := "/open-apis/docx/v1/documents/" + url.PathEscape(docToken) + "/blocks/" + url.PathEscape(docToken) + "/children"
	body := map[string]any{"children": []any{map[string]any{"block_type": 2, "text": map[string]any{"elements": []any{map[string]any{"text_run": map[string]any{"content": title, "text_element_style": map[string]any{"link": map[string]any{"url": link}}}}}}}}}
	var out struct {
		Children []struct {
			ID string `json:"block_id"`
		} `json:"children"`
	}
	if err := s.client.Call(ctx, http.MethodPost, path, body, &out); err != nil {
		return err
	}
	if len(out.Children) != 1 || out.Children[0].ID == "" {
		return errNeedsAttention
	}
	_, err := s.db.ExecContext(ctx, `UPDATE cooperation_feishu_target SET doc_block_id=? WHERE cooperation_id=?`, out.Children[0].ID, cooperationID)
	return err
}

func fields() []map[string]any {
	names := []string{"申请ID", "用户ID", "申请轮次", "审核状态", "申请时间", "审核时间", "审核备注", "昵称", "学校", "专业", "入学年份", "手机", "微信", "邮箱", "技能", "MBTI", "自我评价", "项目经历", "协作分"}
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		out = append(out, map[string]any{"field_name": name, "type": 1})
	}
	return out
}
