package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/oss"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

type CampusCooperationService struct {
	repo              *repository.Repository
	message           *MessageService
	commons           *CommonsService
	dispatchSubscribe func(int64)
}

func NewCampusCooperationService(repo *repository.Repository, message *MessageService, commons *CommonsService) *CampusCooperationService {
	s := &CampusCooperationService{repo: repo, message: message, commons: commons}
	if message != nil {
		s.dispatchSubscribe = message.DispatchCommittedSubscribe
	}
	return s
}

type CommercialOrderInput struct {
	Name        string  `json:"name"`
	CompanyName string  `json:"companyName"`
	Description *string `json:"description"`
	Status      string  `json:"status"`
}
type CooperationContentInput struct {
	Title               string                      `json:"title"`
	Introduction        string                      `json:"introduction"`
	LinksVisible        bool                        `json:"linksVisible"`
	Links               []models.CooperationLink    `json:"links"`
	ImagesVisible       bool                        `json:"imagesVisible"`
	ImageTitle          *string                     `json:"imageTitle"`
	Images              []string                    `json:"images"`
	ContactsVisible     *bool                       `json:"contactsVisible"`
	Contacts            []models.CooperationContact `json:"contacts"`
	ApplicationRequired bool                        `json:"applicationRequired"`
	Published           bool                        `json:"published"`
}

func (in *CommercialOrderInput) Validate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.CompanyName = strings.TrimSpace(in.CompanyName)
	if in.Status == "" {
		in.Status = "available"
	}
	if in.Description != nil && len(*in.Description) > 65535 {
		return ErrBadRequest("商单说明超出文本存储长度")
	}
	if in.Name == "" || in.CompanyName == "" || utf8.RuneCountInString(in.Name) > 100 || utf8.RuneCountInString(in.CompanyName) > 100 {
		return ErrBadRequest("商单名称和公司名称必填，且各不超过100字")
	}
	if in.Status != "available" && in.Status != "closed" {
		return ErrBadRequest("商单状态无效")
	}
	return nil
}
func (in *CooperationContentInput) Validate() error {
	in.Title = strings.TrimSpace(in.Title)
	in.Introduction = strings.TrimSpace(in.Introduction)
	if len(in.Introduction) > 65535 {
		return ErrBadRequest("文字简介超出文本存储长度")
	}
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 20 || in.Introduction == "" {
		return ErrBadRequest("标题和文字简介必填，标题最多20字")
	}
	if in.ImageTitle != nil && utf8.RuneCountInString(*in.ImageTitle) > 100 {
		return ErrBadRequest("图片框名称最多100字")
	}
	for i := range in.Links {
		link := &in.Links[i]
		link.Title = strings.TrimSpace(link.Title)
		link.URL = strings.TrimSpace(link.URL)
		u, err := url.Parse(link.URL)
		if link.Title == "" || err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
			return ErrBadRequest("每个链接都需要标题及有效的HTTP/HTTPS地址")
		}
		if link.Public == nil {
			return ErrBadRequest("请刷新后台，为每条链接设置是否无需申请即可查看")
		}
	}
	for _, contact := range in.Contacts {
		switch contact.Type {
		case "qq_group", "wechat", "qq", "email":
		default:
			return ErrBadRequest("联系方式类型无效")
		}
		if strings.TrimSpace(contact.Value) == "" {
			return ErrBadRequest("已添加的联系方式不能为空")
		}
	}
	seen := map[string]bool{}
	for _, key := range in.Images {
		if !strings.HasPrefix(key, "campus-cooperation-images/") || strings.Contains(key, "..") || len(key) > 255 || seen[key] {
			return ErrBadRequest("图片标识无效或重复")
		}
		seen[key] = true
	}
	return nil
}

func cooperationError(err error) error {
	if err == nil {
		return nil
	}
	var business *ServiceError
	if errors.As(err, &business) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound("合作资源不存在或不可访问")
	}
	return err
}
func (s *CampusCooperationService) adminScope(ctx context.Context, q sqlx.QueryerContext, adminID int, lock bool) (repository.CooperationAdminScope, error) {
	scope, err := s.repo.CampusCooperation.AdminScope(ctx, q, adminID, lock)
	if errors.Is(err, sql.ErrNoRows) {
		return scope, ErrForbidden("当前管理员无合作业务权限")
	}
	return scope, err
}
func (s *CampusCooperationService) AdminOrders(ctx context.Context, adminID, page, size int, status string) ([]models.CommercialOrder, int64, error) {
	scope, err := s.adminScope(ctx, s.repo.DB(), adminID, false)
	if err != nil {
		return nil, 0, err
	}
	if scope.Role != models.AdminRoleSuperAdmin && (scope.Role != models.AdminRoleSchoolSuperAdmin || len(scope.SchoolIDs) == 0) {
		return nil, 0, ErrForbidden("只有平台超管或当前校区负责人可进入单据大厅")
	}
	if status != "" && status != "available" && status != "closed" {
		return nil, 0, ErrBadRequest("商单状态无效")
	}
	return s.repo.CampusCooperation.ListOrders(ctx, page, size, status)
}
func (s *CampusCooperationService) AdminList(ctx context.Context, adminID, page, size int) ([]models.CampusCooperation, int64, error) {
	scope, err := s.adminScope(ctx, s.repo.DB(), adminID, false)
	if err != nil {
		return nil, 0, err
	}
	ids := scope.SchoolIDs
	if scope.Role == models.AdminRoleSuperAdmin {
		ids = nil
	}
	return s.repo.CampusCooperation.List(ctx, ids, false, page, size)
}
func (s *CampusCooperationService) AdminDetail(ctx context.Context, adminID int, id int64, edit bool) (*models.CampusCooperation, error) {
	scope, err := s.adminScope(ctx, s.repo.DB(), adminID, false)
	if err != nil {
		return nil, err
	}
	c, err := repository.GetCampusCooperation(ctx, s.repo.DB(), id, false)
	if err != nil {
		return nil, cooperationError(err)
	}
	if !scope.CanAccess(c.SchoolID, edit) {
		return nil, ErrNotFound("合作资源不存在或不可访问")
	}
	return c, nil
}
func (s *CampusCooperationService) AdminApplications(ctx context.Context, adminID int, id int64, page, size int) ([]models.CooperationApplication, int64, error) {
	if _, err := s.AdminDetail(ctx, adminID, id, false); err != nil {
		return nil, 0, err
	}
	return s.repo.CampusCooperation.ListApplications(ctx, id, 0, page, size)
}

type CooperationPublicView struct {
	ID                  int64                       `json:"id"`
	OrderName           string                      `json:"orderName"`
	CompanyName         string                      `json:"companyName"`
	SchoolName          string                      `json:"schoolName"`
	Title               *string                     `json:"title"`
	Introduction        *string                     `json:"introduction"`
	LinksVisible        bool                        `json:"linksVisible"`
	Links               []models.CooperationLink    `json:"links"`
	ImagesVisible       bool                        `json:"imagesVisible"`
	ImageTitle          *string                     `json:"imageTitle"`
	Images              []string                    `json:"images"`
	ContactsVisible     bool                        `json:"contactsVisible"`
	Contacts            []models.CooperationContact `json:"contacts,omitempty"`
	ApplicationRequired bool                        `json:"applicationRequired"`
	ApplicationStatus   string                      `json:"applicationStatus"`
	CanApply            bool                        `json:"canApply"`
}

func cooperationPublic(c *models.CampusCooperation, status string) CooperationPublicView {
	v := CooperationPublicView{ID: c.ID, OrderName: c.OrderName, CompanyName: c.CompanyName, SchoolName: c.SchoolName, Title: c.Title, Introduction: c.Introduction,
		Links: []models.CooperationLink{}, ImagesVisible: c.ImagesVisible, ImageTitle: c.ImageTitle, Images: []string{},
		ContactsVisible: c.ContactsVisible && (!c.ApplicationRequired || status == "approved"), ApplicationRequired: c.ApplicationRequired, ApplicationStatus: status,
		CanApply: c.ApplicationRequired && (status == "" || status == "rejected")}
	for _, link := range c.Links {
		public := c.LinksVisible
		if link.Public != nil {
			public = *link.Public
		}
		if public || !c.ApplicationRequired || status == "approved" {
			v.Links = append(v.Links, link)
		}
	}
	// Retain the response flag for existing clients; it describes the filtered list.
	v.LinksVisible = len(v.Links) > 0
	if c.ImagesVisible {
		for _, key := range c.Images {
			v.Images = append(v.Images, oss.FullURL(key))
		}
	}
	if v.ContactsVisible {
		v.Contacts = c.Contacts
	}
	return v
}

const operationLeaderArticle = "https://mp.weixin.qq.com/s/Lyj3_jSEBNnEHur9dLWGYw"

type CooperationClientList struct {
	State                     string                  `json:"state"`
	SchoolID                  *int                    `json:"schoolId"`
	SchoolName                *string                 `json:"schoolName"`
	OperationLeaderArticleURL string                  `json:"operationLeaderArticleUrl"`
	List                      []CooperationPublicView `json:"list"`
	Total                     int64                   `json:"total"`
	Page                      int                     `json:"page"`
	Size                      int                     `json:"size"`
}

func (s *CampusCooperationService) ClientList(ctx context.Context, userID, page, size int) (*CooperationClientList, error) {
	user, err := s.repo.User.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrNotFound("用户不存在")
	}
	out := &CooperationClientList{State: "school-required", SchoolID: user.SchoolID, SchoolName: user.SchoolName, OperationLeaderArticleURL: operationLeaderArticle, List: []CooperationPublicView{}, Page: page, Size: size}
	if user.SchoolID == nil {
		return out, nil
	}
	team, err := repository.HasCooperationTeam(ctx, s.repo.DB(), *user.SchoolID)
	if err != nil {
		return nil, err
	}
	if !team {
		out.State = "no-team"
		return out, nil
	}
	out.State = "ready"
	rows, total, err := s.repo.CampusCooperation.List(ctx, []int{*user.SchoolID}, true, page, size)
	if err != nil {
		return nil, err
	}
	out.Total = total
	for i := range rows {
		a, err := repository.LatestCooperationApplication(ctx, s.repo.DB(), rows[i].ID, userID, false)
		if err != nil {
			return nil, err
		}
		status := ""
		if a != nil {
			status = a.Status
		}
		out.List = append(out.List, cooperationPublic(&rows[i], status))
	}
	return out, nil
}
func (s *CampusCooperationService) ClientDetail(ctx context.Context, userID int, id int64) (*CooperationPublicView, error) {
	user, err := s.repo.User.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	c, err := repository.GetCampusCooperation(ctx, s.repo.DB(), id, false)
	if err != nil {
		return nil, cooperationError(err)
	}
	if user == nil || user.SchoolID == nil || *user.SchoolID != c.SchoolID || !c.Published {
		return nil, ErrNotFound("合作资源不存在或不可访问")
	}
	team, err := repository.HasCooperationTeam(ctx, s.repo.DB(), c.SchoolID)
	if err != nil {
		return nil, err
	}
	if !team {
		return nil, ErrNotFound("学校暂无运营团队")
	}
	a, err := repository.LatestCooperationApplication(ctx, s.repo.DB(), id, userID, false)
	if err != nil {
		return nil, err
	}
	status := ""
	if a != nil {
		status = a.Status
	}
	out := cooperationPublic(c, status)
	return &out, nil
}
func (s *CampusCooperationService) MyApplications(ctx context.Context, userID, page, size int) ([]models.CooperationApplication, int64, error) {
	return s.repo.CampusCooperation.ListApplications(ctx, 0, userID, page, size)
}

func (s *CampusCooperationService) SaveOrder(ctx context.Context, adminID int, id int64, in CommercialOrderInput) (int64, error) {
	if err := in.Validate(); err != nil {
		return 0, err
	}
	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	scope, err := s.adminScope(ctx, tx, adminID, true)
	if err != nil {
		return 0, err
	}
	if scope.Role != models.AdminRoleSuperAdmin {
		return 0, ErrForbidden("只有平台超管可以录入或更新商单")
	}
	if id == 0 {
		result, err := tx.ExecContext(ctx, `INSERT INTO commercial_order(name,company_name,description,status,created_by,updated_by) VALUES(?,?,?,?,?,?)`, in.Name, in.CompanyName, in.Description, in.Status, adminID, adminID)
		if err != nil {
			return 0, err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return 0, err
		}
	} else {
		var existing int64
		if err := tx.GetContext(ctx, &existing, `SELECT id FROM commercial_order WHERE id=? FOR UPDATE`, id); err != nil {
			return 0, cooperationError(err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE commercial_order SET name=?,company_name=?,description=?,status=?,updated_by=? WHERE id=?`, in.Name, in.CompanyName, in.Description, in.Status, adminID, id); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}
func (s *CampusCooperationService) Claim(ctx context.Context, adminID, schoolID int, orderID int64) (*models.CampusCooperation, error) {
	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	scope, err := s.adminScope(ctx, tx, adminID, true)
	if err != nil {
		return nil, err
	}
	if scope.Role != models.AdminRoleSchoolSuperAdmin || !scope.CanAccess(schoolID, true) {
		return nil, ErrForbidden("只有该校当前唯一负责人可以接单")
	}
	var status string
	if err := tx.GetContext(ctx, &status, `SELECT status FROM commercial_order WHERE id=? FOR UPDATE`, orderID); err != nil {
		return nil, cooperationError(err)
	}
	var id int64
	err = tx.GetContext(ctx, &id, `SELECT id FROM campus_cooperation WHERE commercial_order_id=? AND school_id=?`, orderID, schoolID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		if status != "available" {
			return nil, ErrBadRequest("该商单已停止接单")
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO campus_cooperation(commercial_order_id,school_id,claimed_by) VALUES(?,?,?)`, orderID, schoolID, adminID)
		if err != nil {
			return nil, err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return nil, err
		}
	}
	c, err := repository.GetCampusCooperation(ctx, tx, id, false)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return c, nil
}
func (s *CampusCooperationService) SaveContent(ctx context.Context, adminID int, id int64, in CooperationContentInput) error {
	if err := in.Validate(); err != nil {
		return err
	}
	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	scope, err := s.adminScope(ctx, tx, adminID, true)
	if err != nil {
		return err
	}
	c, err := repository.GetCampusCooperation(ctx, tx, id, true)
	if err != nil {
		return cooperationError(err)
	}
	if !scope.CanAccess(c.SchoolID, true) {
		return ErrNotFound("合作资源不存在或不可编辑")
	}
	for _, key := range in.Images {
		var registered string
		if err := tx.GetContext(ctx, &registered, `SELECT object_key FROM campus_cooperation_image WHERE object_key=? AND cooperation_id=? FOR UPDATE`, key, id); err != nil {
			return ErrBadRequest("图片不存在或不属于该校合作")
		}
	}
	links, err := json.Marshal(in.Links)
	if err != nil {
		return err
	}
	images, err := json.Marshal(in.Images)
	if err != nil {
		return err
	}
	contacts, err := json.Marshal(in.Contacts)
	if err != nil {
		return err
	}
	visible := true
	if in.ContactsVisible != nil {
		visible = *in.ContactsVisible
	}
	_, err = tx.ExecContext(ctx, `UPDATE campus_cooperation SET title=?,introduction=?,links_visible=?,links=?,images_visible=?,image_title=?,images=?,contacts_visible=?,contacts=?,application_required=?,published=?,updated_by=? WHERE id=?`, in.Title, in.Introduction, in.LinksVisible, string(links), in.ImagesVisible, in.ImageTitle, string(images), visible, string(contacts), in.ApplicationRequired, in.Published, adminID, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *CampusCooperationService) Apply(ctx context.Context, userID int, id int64) (*models.CooperationApplication, error) {
	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Lock this school opportunity before checking the last attempt: concurrent taps
	// cannot create two attempts. The active-user unique key is a second fence.
	c, err := repository.GetCampusCooperation(ctx, tx, id, true)
	if err != nil {
		return nil, cooperationError(err)
	}
	var user struct {
		SchoolID *int `db:"school_id"`
		Status   int  `db:"user_status"`
	}
	if err := tx.GetContext(ctx, &user, "SELECT school_id,user_status FROM `user` WHERE id=? FOR UPDATE", userID); err != nil {
		return nil, cooperationError(err)
	}
	if user.Status != 0 || user.SchoolID == nil || *user.SchoolID != c.SchoolID || !c.Published {
		return nil, ErrNotFound("合作资源不存在或不可申请")
	}
	team, err := repository.HasCooperationTeam(ctx, tx, c.SchoolID)
	if err != nil {
		return nil, err
	}
	if !team {
		return nil, ErrBadRequest("学校暂无运营团队")
	}
	if !c.ApplicationRequired {
		return nil, ErrBadRequest("该合作不需要申请")
	}
	last, err := repository.LatestCooperationApplication(ctx, tx, id, userID, true)
	if err != nil {
		return nil, err
	}
	attempt := 1
	if last != nil {
		if last.Status == "pending" || last.Status == "approved" {
			last.ProfileJSON = nil
			last.ReviewRemark = nil
			last.ReviewedBy = nil
			return last, tx.Commit()
		}
		attempt = last.AttemptNo + 1
	}
	// Whitelist only fields shown by the existing card confirmation flow. Never
	// snapshot OpenID, credentials, certification images or financial fields.
	var snapshot string
	err = tx.GetContext(ctx, &snapshot, `SELECT JSON_OBJECT('userId',u.id,'nickname',u.nickname,'schoolId',u.school_id,'schoolName',s.school_name,'majorName',m.major_name,'grade',u.grade,'avatarUrl',u.avatar_url,'phone',u.phone,'email',u.email,'wechat',u.wechat_id,'authStatus',u.auth_status,'collaborationScore',u.collaboration_score,'skills',JSON_EXTRACT(tp.skill_summary,'$'),'mbti',tp.mbti,'selfEvaluation',tp.self_evaluation,'projectExperience',tp.project_experience)
 FROM `+"`user`"+` u LEFT JOIN school s ON s.id=u.school_id LEFT JOIN major m ON m.id=u.major_id LEFT JOIN talent_profile tp ON tp.user_id=u.id WHERE u.id=?`, userID)
	if err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO campus_cooperation_application(cooperation_id,user_id,attempt_no,profile_snapshot) VALUES(?,?,?,?)`, id, userID, attempt, snapshot)
	if err != nil {
		return nil, err
	}
	appID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	var a models.CooperationApplication
	if err := tx.GetContext(ctx, &a, `SELECT id,cooperation_id,user_id,attempt_no,status,applied_at FROM campus_cooperation_application WHERE id=?`, appID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *CampusCooperationService) Review(ctx context.Context, adminID int, applicationID int64, status string, remark *string) error {
	if status != "approved" && status != "rejected" {
		return ErrBadRequest("审核状态只能是approved或rejected")
	}
	if remark != nil && utf8.RuneCountInString(*remark) > 500 {
		return ErrBadRequest("审核备注最多500字")
	}
	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	scope, err := s.adminScope(ctx, tx, adminID, true)
	if err != nil {
		return err
	}
	var cooperationID int64
	if err := tx.GetContext(ctx, &cooperationID, `SELECT cooperation_id FROM campus_cooperation_application WHERE id=?`, applicationID); err != nil {
		return cooperationError(err)
	}
	c, err := repository.GetCampusCooperation(ctx, tx, cooperationID, true)
	if err != nil {
		return cooperationError(err)
	}
	if !scope.CanAccess(c.SchoolID, false) {
		return ErrNotFound("申请不存在或不可审核")
	}
	var a models.CooperationApplication
	if err := tx.GetContext(ctx, &a, `SELECT id,cooperation_id,user_id,attempt_no,status,applied_at FROM campus_cooperation_application WHERE id=? FOR UPDATE`, applicationID); err != nil {
		return cooperationError(err)
	}
	if a.Status != "pending" {
		if a.Status == status {
			return tx.Commit()
		}
		return ErrBadRequest("该申请已审核，请刷新列表")
	}
	now := time.Now()
	a.Status = status
	a.ReviewedAt = &now
	result, err := tx.ExecContext(ctx, `UPDATE campus_cooperation_application SET status=?,reviewed_by=?,reviewed_at=?,review_remark=? WHERE id=? AND status='pending'`, status, adminID, now, remark, applicationID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrBadRequest("该申请已审核，请刷新列表")
	}
	if _, err := repository.CooperationResultTx(ctx, tx, &a, c); err != nil {
		return err
	}
	deliveryID := int64(0)
	if status == "approved" && s.message != nil {
		title := "校园合作"
		if c.Title != nil {
			title = *c.Title
		}
		deliveryID, err = s.message.QueueSubscribeTxWithPage(ctx, tx, a.UserID, models.MsgBizKeyCampusCooperationResult, map[string]string{"application": truncate20(title), "result": "申请通过", "remark": "点击查看联系方式"}, fmt.Sprintf("pages/campus-cooperation-detail/campus-cooperation-detail?id=%d", c.ID))
		if err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if deliveryID > 0 {
		s.dispatchSubscribe(deliveryID)
	}
	return nil
}

func (s *CampusCooperationService) UploadImage(ctx context.Context, adminID int, id int64, file multipart.File, header *multipart.FileHeader) (*oss.UploadResult, error) {
	if _, err := s.AdminDetail(ctx, adminID, id, true); err != nil {
		return nil, err
	}
	result, err := s.commons.UploadCooperationImage(file, header)
	if err != nil {
		return nil, err
	}
	// Recheck current ownership after the upload. A handover during upload must not
	// allow the former operator to attach images to a school they no longer own.
	tx, err := s.repo.DB().BeginTxx(ctx, nil)
	if err != nil {
		_ = s.commons.DeleteFile(result.Key)
		return nil, err
	}
	defer tx.Rollback()
	scope, err := s.adminScope(ctx, tx, adminID, true)
	if err == nil {
		var c *models.CampusCooperation
		c, err = repository.GetCampusCooperation(ctx, tx, id, true)
		if err == nil && !scope.CanAccess(c.SchoolID, true) {
			err = ErrForbidden("学校负责人已变更，无法上传")
		}
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO campus_cooperation_image(object_key,cooperation_id,uploaded_by) VALUES(?,?,?)`, result.Key, id, adminID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		_ = s.commons.DeleteFile(result.Key)
		return nil, cooperationError(err)
	}
	return result, nil
}
