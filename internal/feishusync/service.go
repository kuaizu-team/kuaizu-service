package feishusync

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"log"
	"net/url"
	"os"
	"strings"
	"time"
)

type PermissionError struct{ Message string }

func (e *PermissionError) Error() string { return e.Message }

type ReconcileError struct{ Message string }

func (e *ReconcileError) Error() string { return e.Message }

type Service struct {
	repo     *repository.Repository
	store    *Store
	remote   Remote
	wikiBase string
	wake     chan struct{}
}

func NewFromEnv(ctx context.Context, repo *repository.Repository) (*Service, error) {
	if os.Getenv("FEISHU_USER_SYNC_ENABLED") != "true" {
		return nil, nil
	}
	app, secret := strings.TrimSpace(os.Getenv("FEISHU_APP_ID")), strings.TrimSpace(os.Getenv("FEISHU_APP_SECRET"))
	space, parent := strings.TrimSpace(os.Getenv("FEISHU_WIKI_SPACE_ID")), strings.TrimSpace(os.Getenv("FEISHU_WIKI_PARENT_NODE_TOKEN"))
	if app == "" || secret == "" || space == "" || parent == "" {
		return nil, fmt.Errorf("飞书同步配置不完整：需要应用凭证、知识库 ID 和父节点 token")
	}
	base := strings.TrimRight(os.Getenv("FEISHU_WIKI_BASE_URL"), "/")
	if base == "" {
		base = "https://feishu.cn"
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || !(u.Host == "feishu.cn" || strings.HasSuffix(u.Host, ".feishu.cn")) {
		return nil, fmt.Errorf("FEISHU_WIKI_BASE_URL 必须为飞书 HTTPS 域名")
	}
	s := &Service{repo: repo, store: NewStore(repo.DB()), remote: NewClient(app, secret, space, parent), wikiBase: base, wake: make(chan struct{}, 1)}
	if err = s.store.CheckSchema(ctx); err != nil {
		return nil, err
	}
	if maximum := repo.DB().Stats().MaxOpenConnections; maximum > 0 && maximum < 2 {
		return nil, fmt.Errorf("飞书同步需要 DB_MAX_OPEN_CONNS 至少为 2，以保留同步锁专用连接")
	}
	return s, nil
}

// Read current DB authorization, including disablement and delegation changes.
// A school is mandatory for super roles; the regular school admin may omit it.
func Authorize(ctx context.Context, repo *repository.Repository, adminID, requested int) (int, error) {
	admin, err := repo.AdminUser.GetAuthStateByID(ctx, adminID)
	if err != nil {
		return 0, err
	}
	if admin == nil || admin.Status != 1 {
		return 0, &PermissionError{"管理员账号已停用或不存在"}
	}
	return authorizeAdmin(ctx, repo, admin, requested)
}
func authorizeAdmin(ctx context.Context, repo *repository.Repository, admin *models.AdminUser, requested int) (int, error) {
	switch admin.Role {
	case models.AdminRoleSuperAdmin:
		if requested <= 0 {
			return 0, &PermissionError{"每次必须选择一个学校，暂不支持全部学校同步"}
		}
		return requested, nil
	case models.AdminRoleSchoolSuperAdmin:
		if requested <= 0 {
			return 0, &PermissionError{"请选择一个授权学校"}
		}
		ids, err := repo.AdminUser.AccessibleSchoolIDs(ctx, admin.ID)
		if err != nil {
			return 0, err
		}
		for _, id := range ids {
			if id == requested {
				return requested, nil
			}
		}
		return 0, &PermissionError{"无该学校的同步权限"}
	case models.AdminRoleSchoolAdmin:
		if admin.SchoolID == nil || *admin.SchoolID <= 0 {
			return 0, &PermissionError{"尚未绑定学校，不能同步"}
		}
		if requested != 0 && requested != *admin.SchoolID {
			return 0, &PermissionError{"只能同步绑定学校的用户"}
		}
		return *admin.SchoolID, nil
	default:
		return 0, &PermissionError{"无飞书同步权限"}
	}
}

func (s *Service) Enqueue(ctx context.Context, adminID, schoolID int) (*Job, error) {
	job, err := s.store.Enqueue(ctx, adminID, schoolID)
	if err != nil {
		return nil, err
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
	s.decorate(ctx, job)
	return job, nil
}
func (s *Service) Latest(ctx context.Context, schoolID int) (*Job, error) {
	job, err := s.store.Latest(ctx, schoolID)
	if err != nil {
		return nil, err
	}
	s.decorate(ctx, job)
	return job, nil
}
func (s *Service) decorate(ctx context.Context, job *Job) {
	if job == nil {
		return
	}
	if target, err := s.store.Target(ctx, job.SchoolID); err == nil && target.NodeToken != "" {
		job.URL = s.wikiBase + "/wiki/" + url.PathEscape(target.NodeToken)
		if target.TableID != "" {
			job.URL += "?table=" + url.QueryEscape(target.TableID) + "&view=" + url.QueryEscape(target.ViewID)
		}
	}
}

func (s *Service) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			if worked := s.workOne(ctx); worked {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-s.wake:
			}
		}
	}()
}

// Dedicated MySQL named lock serializes all Feishu writes across admin instances.
// It is released when the connection dies. Check ownership before each mutation;
// abandoned running jobs are failed conservatively instead of replaying creates.
const workerLock = "kuaizu:feishu:user-sync:v1"

func (s *Service) workOne(root context.Context) bool {
	ctx, cancel := context.WithCancel(root)
	defer cancel()
	conn, err := s.repo.DB().DB.Conn(ctx)
	if err != nil {
		return false
	}
	defer conn.Close()
	var acquired sql.NullInt64
	if err = conn.QueryRowContext(ctx, "SELECT GET_LOCK(?,0)", workerLock).Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
		return false
	}
	defer func() {
		release, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		var result sql.NullInt64
		if err := conn.QueryRowContext(release, "SELECT RELEASE_LOCK(?)", workerLock).Scan(&result); err != nil || !result.Valid || result.Int64 != 1 {
			// Close() normally returns the session to the pool; an unreleased lock
			// must instead discard the physical connection.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	job, err := s.store.Next(ctx)
	if err != nil || job == nil {
		return false
	}
	guard := func() error {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("同步被中断或超时，请重新同步")
		}
		var owns int
		if err := conn.QueryRowContext(ctx, "SELECT COALESCE(IS_USED_LOCK(?)=CONNECTION_ID(),0)", workerLock).Scan(&owns); err != nil || owns != 1 {
			return fmt.Errorf("同步锁已失效，任务停止")
		}
		_, err := Authorize(ctx, s.repo, job.AdminID, job.SchoolID)
		var permission *PermissionError
		if err != nil && !errors.As(err, &permission) {
			return fmt.Errorf("重新校验管理员权限失败，任务停止")
		}
		return err
	}
	if job.Status == "running" {
		job.Status, job.Message = "failed", "上次同步进程已中断。可重新同步；如有未确认的新增记录，将暂停并提示核对。"
	} else {
		job.Status, job.Message = "running", "正在准备学校表格"
		err = s.store.Progress(ctx, job)
		if err == nil {
			err = s.syncSchool(ctx, job, guard)
		}
		if err == nil {
			job.Status, job.Message = "succeeded", "同步完成"
		} else {
			job.Status = "failed"
			var reconcile *ReconcileError
			var permission *PermissionError
			var api *APIError
			switch {
			case errors.As(err, &reconcile):
				job.Status, job.Message = "needs_attention", reconcile.Message
			case errors.As(err, &permission):
				job.Message = permission.Message
			case errors.As(err, &api):
				job.Message = api.Error()
			default:
				job.Message = "同步失败：" + safeError(err)
			}
		}
	}
	finish, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err := s.store.Finish(finish, job); err != nil {
		log.Printf("feishu sync job %s status persistence failed", job.ID)
		return false
	}
	return true
}

func safeError(err error) string {
	// SQL errors may contain user values. Only application-defined messages are
	// returned by syncSchool; database failures are wrapped in fixed messages.
	text := err.Error()
	if len([]rune(text)) > 400 {
		return "服务异常，请检查服务日志与同步配置"
	}
	return text
}

func (s *Service) provision(ctx context.Context, job *Job, guard func() error) (Target, error) {
	target, err := s.store.Target(ctx, job.SchoolID)
	if err != nil {
		return target, fmt.Errorf("读取学校表格配置失败")
	}
	if target.AppToken == "" {
		if target.NodeStarted {
			return target, &ReconcileError{"知识库节点创建结果待核对，请管理员核对并补全该学校的节点映射，系统不会重复创建。"}
		}
		school, err := s.repo.School.GetByID(ctx, job.SchoolID)
		if err != nil || school == nil {
			return target, fmt.Errorf("学校不存在或读取学校失败")
		}
		if err = guard(); err != nil {
			return target, err
		}
		target.NodeStarted = true
		if err = s.store.SaveTarget(ctx, target); err != nil {
			return target, fmt.Errorf("保存知识库创建状态失败")
		}
		target.NodeToken, target.AppToken, err = s.remote.CreateNode(ctx, fmt.Sprintf("%s—用户名单（学校%d）", school.SchoolName, school.ID))
		if err != nil {
			return target, &ReconcileError{"知识库节点创建结果待核对；请检查应用权限和首页授权并核对节点，系统不会重复创建。"}
		}
		if err = s.store.SaveTarget(ctx, target); err != nil {
			return target, &ReconcileError{"知识库节点已创建，但本地映射保存失败，请管理员补全映射。"}
		}
	}
	if target.TableID == "" {
		if target.TableStarted {
			return target, &ReconcileError{"用户名单数据表创建结果待核对，请管理员补全表格映射，系统不会重复创建。"}
		}
		if err = guard(); err != nil {
			return target, err
		}
		target.TableStarted = true
		if err = s.store.SaveTarget(ctx, target); err != nil {
			return target, fmt.Errorf("保存数据表创建状态失败")
		}
		target.TableID, target.ViewID, err = s.remote.CreateTable(ctx, target)
		if err != nil {
			return target, &ReconcileError{"用户名单数据表创建结果待核对，请核对表格并补全映射，系统不会重复创建。"}
		}
		if err = s.store.SaveTarget(ctx, target); err != nil {
			return target, &ReconcileError{"用户名单数据表已创建，但本地映射保存失败，请管理员补全映射。"}
		}
	}
	fields, err := s.remote.GetFields(ctx, target)
	if err != nil {
		return target, err
	}
	return target, ValidateFields(fields)
}

func (s *Service) syncSchool(ctx context.Context, job *Job, guard func() error) error {
	if err := guard(); err != nil {
		return err
	}
	target, err := s.provision(ctx, job, guard)
	if err != nil {
		return err
	}
	if err = s.cleanup(ctx, target, job, guard); err != nil {
		return err
	}
	ids, err := s.store.UserIDs(ctx, job.SchoolID)
	if err != nil {
		return fmt.Errorf("查询本校用户失败")
	}
	job.Total, job.Message = len(ids), "正在同步本校全部用户"
	if err = s.store.Progress(ctx, job); err != nil {
		return fmt.Errorf("保存同步进度失败")
	}
	for _, id := range ids {
		if err = guard(); err != nil {
			return err
		}
		row, err := s.store.User(ctx, job.SchoolID, id)
		if err != nil {
			return fmt.Errorf("读取本校用户字段失败")
		}
		if row != nil {
			fields, err := row.Values()
			if err != nil {
				return err
			}
			action, err := s.syncRecord(ctx, target, id, fields, guard)
			if err != nil {
				return err
			}
			if action == "created" {
				job.Created++
			} else if action == "updated" {
				job.Updated++
			}
		}
		job.Processed++
		if err = s.store.Progress(ctx, job); err != nil {
			return fmt.Errorf("保存同步进度失败")
		}
	}
	// Recheck users transferred/deleted while this job was running.
	return s.cleanup(ctx, target, job, guard)
}

func (s *Service) syncRecord(ctx context.Context, target Target, id int, fields map[string]any, guard func() error) (string, error) {
	record, err := s.store.Record(ctx, target.SchoolID, id)
	if err != nil {
		return "", fmt.Errorf("读取用户记录映射失败")
	}
	if record != nil {
		if record.State != "ready" || record.RecordID == "" {
			return "", &ReconcileError{fmt.Sprintf("用户 %d 的飞书新增结果待核对，请管理员核对记录映射后重试；系统不会重新插入。", id)}
		}
		if err = guard(); err != nil {
			return "", err
		}
		belongs, err := s.store.Belongs(ctx, target.SchoolID, id)
		if err != nil {
			return "", fmt.Errorf("校验用户学校归属失败")
		}
		if !belongs {
			return "skipped", nil
		}
		err = s.remote.UpdateRecord(ctx, target, record.RecordID, fields)
		if err == nil {
			return "updated", nil
		}
		if !isMissingRecord(err) {
			return "", err
		}
		// A definite RecordIdNotFound allows replacing only this managed mapping.
		if err = s.store.Forget(ctx, target.SchoolID, id); err != nil {
			return "", fmt.Errorf("清理失效记录映射失败")
		}
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		return "", fmt.Errorf("用户字段编码失败")
	}
	if err = guard(); err != nil {
		return "", err
	}
	belongs, err := s.store.Belongs(ctx, target.SchoolID, id)
	if err != nil {
		return "", fmt.Errorf("校验用户学校归属失败")
	}
	if !belongs {
		return "skipped", nil
	}
	token := uuid.NewString()
	if err = s.store.PrepareCreate(ctx, target.SchoolID, id, token, string(payload)); err != nil {
		return "", fmt.Errorf("保存新增记录请求失败")
	}
	recordID, err := s.remote.CreateRecord(ctx, target, token, fields)
	if err != nil {
		return "", &ReconcileError{fmt.Sprintf("用户 %d 的飞书新增结果待核对，请核对应用权限、网络及本地请求记录，系统不会重复插入。", id)}
	}
	if err = s.store.ConfirmCreate(ctx, target.SchoolID, id, recordID); err != nil {
		return "", &ReconcileError{fmt.Sprintf("用户 %d 的飞书记录已创建，但映射保存失败，请管理员补全映射。", id)}
	}
	return "created", nil
}
func (s *Service) cleanup(ctx context.Context, target Target, job *Job, guard func() error) error {
	records, err := s.store.Stale(ctx, target.SchoolID)
	if err != nil {
		return fmt.Errorf("查询本校失效记录映射失败")
	}
	for _, record := range records {
		if record.State != "ready" || record.RecordID == "" {
			return &ReconcileError{fmt.Sprintf("已离校或删除用户 %d 的新增结果待核对，请先核对本地记录映射。", record.UserID)}
		}
		if err = guard(); err != nil {
			return err
		}
		belongs, err := s.store.Belongs(ctx, target.SchoolID, record.UserID)
		if err != nil {
			return fmt.Errorf("校验失效记录学校归属失败")
		}
		if belongs {
			continue
		}
		if err = s.remote.DeleteRecord(ctx, target, record.RecordID); err != nil && !isMissingRecord(err) {
			return err
		}
		if err = s.store.Forget(ctx, target.SchoolID, record.UserID); err != nil {
			return fmt.Errorf("清理失效记录映射失败")
		}
		job.Deleted++
		if err = s.store.Progress(ctx, job); err != nil {
			return fmt.Errorf("保存同步进度失败")
		}
	}
	return nil
}
