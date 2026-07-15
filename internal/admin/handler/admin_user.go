package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	adminvo "github.com/kuaizu-team/kuaizu-service/internal/admin/vo"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

const (
	adminCenterForbiddenMessage     = "校区管理员没有管理员中心权限"
	schoolSuperAdminNoSchoolMessage = "当前校区超级管理员账号未绑定 schoolId，不能操作管理员账号"
)

// ListAdmins handles GET /admin/admins
func (s *AdminServer) ListAdmins(ctx echo.Context) error {
	callerRole := adminRole(ctx)
	callerID := currentAdminID(ctx)

	if callerRole == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, adminCenterForbiddenMessage)
	}

	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("pageSize"))
	if size == 0 {
		size, _ = strconv.Atoi(ctx.QueryParam("size"))
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}

	params := repository.AdminUserListParams{Page: page, Size: size}

	if v := ctx.QueryParam("keyword"); v != "" {
		params.Keyword = &v
	}
	if v := ctx.QueryParam("status"); v != "" {
		st, err := strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid status")
		}
		params.Status = &st
	}

	switch callerRole {
	case models.AdminRoleSuperAdmin:
		if v := ctx.QueryParam("role"); v != "" {
			r, err := strconv.Atoi(v)
			if err != nil {
				return response.BadRequest(ctx, "invalid role")
			}
			params.Role = &r
		}
	case models.AdminRoleSchoolSuperAdmin:
		schoolIDs, err := s.adminSchoolIDs(ctx)
		if err != nil {
			return response.InternalError(ctx, "查询管理员学校权限失败")
		}
		if len(schoolIDs) == 0 {
			return response.Forbidden(ctx, schoolSuperAdminNoSchoolMessage)
		}
		params.SchoolIDs = schoolIDs
		params.ViewerAdminID = &callerID
		params.IncludeAllEventManagers = false
	}

	admins, total, err := s.repo.AdminUser.List(ctx.Request().Context(), params)
	if err != nil {
		return response.InternalError(ctx, "获取管理员列表失败")
	}

	list := make([]adminvo.AdminUserAccountVO, len(admins))
	for i, a := range admins {
		vo := adminvo.NewAdminUserAccountVO(a)
		if s.canViewAdminPasswordInScope(ctx, a) {
			attachAdminPassword(vo, a)
		}
		s.enrichAdminFinance(ctx, vo, a, callerRole == models.AdminRoleSuperAdmin)
		list[i] = *vo
	}

	return response.Success(ctx, map[string]interface{}{
		"list":  list,
		"total": total,
	})
}

func (s *AdminServer) GetAdmin(ctx echo.Context) error {
	callerRole := adminRole(ctx)

	if callerRole == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, adminCenterForbiddenMessage)
	}

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid admin id")
	}
	target, err := s.repo.AdminUser.GetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "查询管理员失败")
	}
	if target == nil {
		return response.NotFound(ctx, "管理员不存在")
	}
	if !s.canViewAdminDetailInScope(ctx, target) {
		return response.Forbidden(ctx, "权限不足")
	}

	vo := adminvo.NewAdminUserAccountVO(target)
	if s.canViewAdminPasswordInScope(ctx, target) {
		attachAdminPassword(vo, target)
	}
	s.enrichAdminFinance(ctx, vo, target, callerRole == models.AdminRoleSuperAdmin)
	return response.Success(ctx, vo)
}

// GetCurrentAdmin handles GET /admin/auth/me.
func (s *AdminServer) GetCurrentAdmin(ctx echo.Context) error {
	callerID := currentAdminID(ctx)
	target, err := s.repo.AdminUser.GetByID(ctx.Request().Context(), callerID)
	if err != nil {
		return response.InternalError(ctx, "查询管理员信息失败")
	}
	if target == nil {
		return response.NotFound(ctx, "管理员不存在")
	}

	vo := adminvo.NewAdminUserAccountVO(target)
	if s.canViewAdminPasswordInScope(ctx, target) {
		attachAdminPassword(vo, target)
	}
	return response.Success(ctx, vo)
}

type updateFinanceRemarkRequest struct {
	FinanceRemark *string `json:"financeRemark"`
}

func (s *AdminServer) UpdateAdminFinanceRemark(ctx echo.Context) error {
	if adminRole(ctx) != models.AdminRoleSuperAdmin {
		return response.Forbidden(ctx, "权限不足")
	}
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid admin id")
	}
	var req updateFinanceRemarkRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	if req.FinanceRemark != nil {
		v := strings.TrimSpace(*req.FinanceRemark)
		req.FinanceRemark = &v
	}
	if err := s.repo.AdminUser.UpdateFinanceRemark(ctx.Request().Context(), id, req.FinanceRemark); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return response.NotFound(ctx, "管理员不存在")
		}
		return response.InternalError(ctx, "更新财务备注失败")
	}
	target, _ := s.repo.AdminUser.GetByID(ctx.Request().Context(), id)
	vo := adminvo.NewAdminUserAccountVO(target)
	s.enrichAdminFinance(ctx, vo, target, true)
	return response.Success(ctx, vo)
}

type updateCommissionRateRequest struct {
	CommissionRate float64 `json:"commissionRate"`
	SchoolID       *int    `json:"schoolId"`
}

func (s *AdminServer) UpdateAdminCommissionRate(ctx echo.Context) error {
	callerRole := adminRole(ctx)

	if callerRole != models.AdminRoleSuperAdmin {
		return response.Forbidden(ctx, "权限不足")
	}

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid admin id")
	}

	var req updateCommissionRateRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	if math.IsNaN(req.CommissionRate) || math.IsInf(req.CommissionRate, 0) || req.CommissionRate < 0 || req.CommissionRate > 100 {
		return response.BadRequest(ctx, "commissionRate must be between 0 and 100")
	}
	req.CommissionRate = math.Round(req.CommissionRate*100) / 100

	target, err := s.repo.AdminUser.GetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "鏌ヨ绠＄悊鍛樺け璐?")
	}
	if target == nil {
		return response.NotFound(ctx, "绠＄悊鍛樹笉瀛樺湪")
	}
	if target.Role == models.AdminRoleSchoolAdmin {
		return response.BadRequest(ctx, "校区管理员不参与分成结算")
	}
	if target.Role == models.AdminRoleSchoolSuperAdmin {
		schoolID := req.SchoolID
		if schoolID == nil && len(target.Schools) == 1 {
			id := target.Schools[0].SchoolID
			schoolID = &id
		}
		if schoolID == nil {
			return response.BadRequest(ctx, "多学校管理员更新分成比例时必须传 schoolId")
		}
		total, err := s.repo.AdminUser.SchoolCommissionTotalExcluding(ctx.Request().Context(), *schoolID, target.ID)
		if err != nil {
			return response.InternalError(ctx, "查询学校分成比例失败")
		}
		if total+req.CommissionRate > 100.000001 {
			return response.BadRequest(ctx, "该学校的分成比例总和不能超过 100%")
		}
		if err := s.repo.AdminUser.UpdateSchoolCommission(ctx.Request().Context(), target.ID, *schoolID, req.CommissionRate); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return response.NotFound(ctx, "管理员未关联该学校")
			}
			return response.InternalError(ctx, "更新学校分成比例失败")
		}
		updated, _ := s.repo.AdminUser.GetByID(ctx.Request().Context(), id)
		vo := adminvo.NewAdminUserAccountVO(updated)
		s.enrichAdminFinance(ctx, vo, updated, true)
		return response.Success(ctx, vo)
	}

	if err := s.repo.AdminUser.UpdateCommissionRate(ctx.Request().Context(), id, req.CommissionRate); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return response.NotFound(ctx, "绠＄悊鍛樹笉瀛樺湪")
		}
		return response.InternalError(ctx, "更新分成比例失败")
	}

	updated, _ := s.repo.AdminUser.GetByID(ctx.Request().Context(), id)
	vo := adminvo.NewAdminUserAccountVO(updated)
	s.enrichAdminFinance(ctx, vo, updated, callerRole == models.AdminRoleSuperAdmin)
	return response.Success(ctx, vo)
}

type settleAdminRequest struct {
	Remark   *string `json:"remark"`
	SchoolID *int    `json:"schoolId"`
}

func (s *AdminServer) settleAdminOrdersLegacy(ctx echo.Context) error {
	if adminRole(ctx) != models.AdminRoleSuperAdmin {
		return response.Forbidden(ctx, "权限不足")
	}
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid admin id")
	}
	target, err := s.repo.AdminUser.GetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "查询管理员失败")
	}
	if target == nil {
		return response.NotFound(ctx, "管理员不存在")
	}
	if target.SchoolID == nil {
		return response.BadRequest(ctx, "管理员未绑定学校，无法一键结算")
	}
	if target.CommissionRate <= 0 {
		return response.BadRequest(ctx, "请先设置大于 0 的分成比例再进行结算")
	}
	var req settleAdminRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	if req.Remark != nil {
		v := strings.TrimSpace(*req.Remark)
		req.Remark = &v
	}
	result, err := s.repo.Order.SettleSchoolPendingOrders(ctx.Request().Context(), *target.SchoolID, currentAdminID(ctx), target.ID, target.CommissionRate, req.Remark)
	if err != nil {
		return response.InternalError(ctx, "一键结算失败")
	}
	return response.Success(ctx, result)
}

func (s *AdminServer) SettleAdminOrders(ctx echo.Context) error {
	if adminRole(ctx) != models.AdminRoleSuperAdmin {
		return response.Forbidden(ctx, "权限不足")
	}
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid admin id")
	}
	target, err := s.repo.AdminUser.GetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "查询管理员失败")
	}
	if target == nil {
		return response.NotFound(ctx, "管理员不存在")
	}
	if target.Role != models.AdminRoleSchoolSuperAdmin {
		return s.settleAdminOrdersLegacy(ctx)
	}

	var req settleAdminRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	if req.Remark != nil {
		value := strings.TrimSpace(*req.Remark)
		req.Remark = &value
	}
	relations, err := s.repo.AdminUser.ListSchoolRelations(ctx.Request().Context(), target.ID, false)
	if err != nil {
		return response.InternalError(ctx, "查询管理员学校结算关系失败")
	}
	batchNos := make([]string, 0, len(relations))
	totalAmount := int64(0)
	orderCount := 0
	matched := false
	for _, relation := range relations {
		if req.SchoolID != nil && relation.SchoolID != *req.SchoolID {
			continue
		}
		matched = true
		if relation.CommissionRate <= 0 {
			continue
		}
		result, err := s.repo.Order.SettleSchoolPendingOrders(ctx.Request().Context(), relation.SchoolID, currentAdminID(ctx), target.ID, relation.CommissionRate, req.Remark)
		if err != nil {
			return response.InternalError(ctx, "结算失败")
		}
		if result.BatchNo != "" {
			batchNos = append(batchNos, result.BatchNo)
		}
		totalAmount += result.TotalAmount
		orderCount += result.OrderCount
	}
	if !matched {
		return response.BadRequest(ctx, "管理员未关联指定学校")
	}
	return response.Success(ctx, map[string]interface{}{
		"batchNos": batchNos, "orderCount": orderCount, "totalAmount": totalAmount,
	})
}

type createAdminRequest struct {
	Username string               `json:"username"`
	Password string               `json:"password"`
	Nickname *string              `json:"nickname"`
	Role     int                  `json:"role"`
	SchoolID *int                 `json:"schoolId"`
	Status   int                  `json:"status"`
	Schools  []adminSchoolRequest `json:"schools"`
}

type delegateAdminRequest struct {
	TargetUserID   *int    `json:"targetUserId"`
	SchoolID       int     `json:"schoolId"`
	CommissionRate float64 `json:"commissionRate"`
	Username       string  `json:"username"`
	Password       string  `json:"password"`
	Nickname       *string `json:"nickname"`
}

// DelegateAdminSchool handles POST /api/v2/admin/delegate and /admin/delegate.
func (s *AdminServer) DelegateAdminSchool(ctx echo.Context) error {
	if adminRole(ctx) != models.AdminRoleSchoolSuperAdmin {
		return response.Forbidden(ctx, "只有校区超级管理员可以分配负责人")
	}
	var req delegateAdminRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	if req.SchoolID <= 0 || math.IsNaN(req.CommissionRate) || math.IsInf(req.CommissionRate, 0) || req.CommissionRate <= 0 || req.CommissionRate > 100 {
		return response.BadRequest(ctx, "schoolId 或 commissionRate 无效")
	}
	req.CommissionRate = math.Round(req.CommissionRate*100) / 100

	var target *models.AdminUser
	if req.TargetUserID == nil {
		if strings.TrimSpace(req.Username) == "" || req.Password == "" {
			return response.BadRequest(ctx, "新管理员账号和密码不能为空")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return response.InternalError(ctx, "密码加密失败")
		}
		encrypted, err := encryptAdminCredential(req.Password)
		if err != nil {
			return response.InternalError(ctx, "密码安全存储失败")
		}
		target = &models.AdminUser{
			Username: strings.TrimSpace(req.Username), PasswordHash: string(hash),
			PasswordEncrypted: &encrypted, Nickname: req.Nickname,
			Role: models.AdminRoleSchoolSuperAdmin, Status: models.AdminUserStatusEnabled,
		}
	}
	targetID, err := s.repo.AdminUser.DelegateSchool(ctx.Request().Context(), currentAdminID(ctx), target, req.TargetUserID, req.SchoolID, req.CommissionRate)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrSchoolNotOwned):
			return response.Forbidden(ctx, "你不是该学校当前负责人")
		case errors.Is(err, repository.ErrCommissionRateExceeded):
			return response.BadRequest(ctx, "分配比例不能超过你在该学校的当前比例")
		case errors.Is(err, repository.ErrDuplicateUsername):
			return response.BadRequest(ctx, "管理员账号已存在")
		case errors.Is(err, repository.ErrInvalidDelegationTarget):
			return response.BadRequest(ctx, "目标管理员无效")
		case errors.Is(err, repository.ErrSchoolAlreadyOwned):
			return response.BadRequest(ctx, "该学校已有其他负责人")
		default:
			return response.InternalError(ctx, "分权失败")
		}
	}
	created, err := s.repo.AdminUser.GetByID(ctx.Request().Context(), targetID)
	if err != nil || created == nil {
		return response.InternalError(ctx, "读取新负责人失败")
	}
	vo := adminvo.NewAdminUserAccountVO(created)
	s.enrichAdminFinance(ctx, vo, created, false)
	return response.Success(ctx, vo)
}

type adminSchoolRequest struct {
	SchoolID       int     `json:"schoolId"`
	CommissionRate float64 `json:"commissionRate"`
}

func validateAdminSchools(schools []adminSchoolRequest) ([]models.AdminSchoolRelation, error) {
	result := make([]models.AdminSchoolRelation, 0, len(schools))
	seen := make(map[int]struct{}, len(schools))
	for _, school := range schools {
		if school.SchoolID <= 0 || math.IsNaN(school.CommissionRate) || math.IsInf(school.CommissionRate, 0) || school.CommissionRate < 0 || school.CommissionRate > 100 {
			return nil, errors.New("schools 中的 schoolId 和 commissionRate 无效")
		}
		if _, exists := seen[school.SchoolID]; exists {
			return nil, errors.New("schools 中不能重复选择学校")
		}
		seen[school.SchoolID] = struct{}{}
		normalizedRate := math.Round(school.CommissionRate*100) / 100
		result = append(result, models.AdminSchoolRelation{SchoolID: school.SchoolID, CommissionRate: normalizedRate, IsOwner: true})
	}
	return result, nil
}

func (s *AdminServer) validateSchoolCommissionCapacity(ctx echo.Context, adminID int, schools []models.AdminSchoolRelation) error {
	for _, school := range schools {
		total, err := s.repo.AdminUser.SchoolCommissionTotalExcluding(ctx.Request().Context(), school.SchoolID, adminID)
		if err != nil {
			return err
		}
		if total+school.CommissionRate > 100.000001 {
			return fmt.Errorf("学校 %d 的分成比例总和不能超过 100%%", school.SchoolID)
		}
	}
	return nil
}

// CreateAdmin handles POST /admin/admins
func (s *AdminServer) CreateAdmin(ctx echo.Context) error {
	callerRole := adminRole(ctx)
	if callerRole == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, adminCenterForbiddenMessage)
	}

	var req createAdminRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}

	if req.Username == "" || req.Password == "" {
		return response.BadRequest(ctx, "username 和 password 不能为空")
	}
	if req.Role < 1 || req.Role > 3 {
		return response.BadRequest(ctx, "role 必须为 1、2 或 3")
	}

	if callerRole == models.AdminRoleSchoolSuperAdmin {
		if req.Role != models.AdminRoleSchoolAdmin {
			return response.Forbidden(ctx, "校区超级管理员只能创建校区管理员")
		}
		schoolIDs, err := s.adminSchoolIDs(ctx)
		if err != nil {
			return response.InternalError(ctx, "查询学校权限失败")
		}
		if len(schoolIDs) == 0 {
			return response.Forbidden(ctx, schoolSuperAdminNoSchoolMessage)
		}
		if req.SchoolID == nil {
			req.SchoolID = &schoolIDs[0]
		}
		if !schoolIDInScope(req.SchoolID, schoolIDs) {
			return response.Forbidden(ctx, "只能在自己负责的学校创建管理员")
		}
	}

	schools, err := validateAdminSchools(req.Schools)
	if err != nil {
		return response.BadRequest(ctx, err.Error())
	}
	if req.Role == models.AdminRoleSchoolSuperAdmin {
		if callerRole != models.AdminRoleSuperAdmin {
			return response.Forbidden(ctx, "只有平台超级管理员可直接绑定多学校")
		}
		if len(schools) == 0 && req.SchoolID != nil {
			schools = []models.AdminSchoolRelation{{SchoolID: *req.SchoolID, CommissionRate: 0, IsOwner: true}}
		}
		if len(schools) == 0 {
			return response.BadRequest(ctx, "校区超级管理员至少需要绑定一个学校")
		}
		req.SchoolID = nil
		if err := s.validateSchoolCommissionCapacity(ctx, 0, schools); err != nil {
			return response.BadRequest(ctx, err.Error())
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return response.InternalError(ctx, "密码加密失败")
	}
	encrypted, err := encryptAdminCredential(req.Password)
	if err != nil {
		return response.InternalError(ctx, "管理员密码安全存储失败")
	}

	admin := &models.AdminUser{
		Username:          req.Username,
		PasswordHash:      string(hash),
		PasswordEncrypted: &encrypted,
		Nickname:          req.Nickname,
		Role:              req.Role,
		SchoolID:          req.SchoolID,
		Status:            req.Status,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if admin.Role == models.AdminRoleSchoolSuperAdmin {
		err = s.repo.AdminUser.CreateWithSchools(ctx.Request().Context(), admin, schools)
	} else {
		err = s.repo.AdminUser.Create(ctx.Request().Context(), admin)
	}
	if err != nil {
		if errors.Is(err, repository.ErrSchoolAlreadyOwned) {
			return response.BadRequest(ctx, "所选学校已有校区超级管理员负责人")
		}
		if errors.Is(err, repository.ErrDuplicateUsername) {
			return response.BadRequest(ctx, "账号已存在")
		}
		return response.InternalError(ctx, "创建管理员失败")
	}

	created, _ := s.repo.AdminUser.GetByID(ctx.Request().Context(), admin.ID)
	return response.Success(ctx, adminvo.NewAdminUserAccountVO(created))
}

type updateAdminRequest struct {
	Nickname   *string               `json:"nickname"`
	Password   string                `json:"password"`
	Role       *int                  `json:"role"`
	SchoolID   **int                 `json:"schoolId"`
	Status     *int                  `json:"status"`
	JoinDate   *string               `json:"joinDate"`
	Intro      *string               `json:"intro"`
	ArticleURL *string               `json:"articleUrl"`
	Schools    *[]adminSchoolRequest `json:"schools"`
}

// UpdateAdmin handles PUT /admin/admins/:id
func (s *AdminServer) UpdateAdmin(ctx echo.Context) error {
	callerRole := adminRole(ctx)
	callerID := currentAdminID(ctx)

	if callerRole == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, adminCenterForbiddenMessage)
	}

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid admin id")
	}

	target, err := s.repo.AdminUser.GetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "查询管理员失败")
	}
	if target == nil {
		return response.NotFound(ctx, "管理员不存在")
	}
	originalRole := target.Role

	if !s.canEditAdminInScope(ctx, target) {
		return response.Forbidden(ctx, "权限不足")
	}

	var req updateAdminRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}

	if callerRole == models.AdminRoleSchoolSuperAdmin && id == callerID && (req.SchoolID != nil || req.Schools != nil) {
		return response.Forbidden(ctx, "校区超级管理员不能修改自己的 schoolId")
	}

	if req.Nickname != nil {
		target.Nickname = req.Nickname
	}
	if req.Role != nil {
		if callerRole == models.AdminRoleSchoolSuperAdmin && *req.Role <= models.AdminRoleSchoolSuperAdmin {
			return response.Forbidden(ctx, "校区超级管理员不能设置超级管理员或校区超级管理员角色")
		}
		if callerRole == models.AdminRoleSuperAdmin && *req.Role == models.AdminRoleSuperAdmin && id != callerID {
			return response.Forbidden(ctx, "不能将他人设置为超级管理员")
		}
		target.Role = *req.Role
	}
	if req.SchoolID != nil {
		target.SchoolID = *req.SchoolID
	}
	ownedSchools := make([]models.AdminSchoolRelation, 0, len(target.Schools))
	for _, school := range target.Schools {
		if school.IsOwner {
			ownedSchools = append(ownedSchools, school)
		}
	}
	if req.Schools != nil {
		if callerRole != models.AdminRoleSuperAdmin {
			return response.Forbidden(ctx, "只有平台超级管理员可以修改多学校绑定")
		}
		parsedSchools, err := validateAdminSchools(*req.Schools)
		if err != nil {
			return response.BadRequest(ctx, err.Error())
		}
		ownedSchools = parsedSchools
	}
	if target.Role == models.AdminRoleSchoolSuperAdmin {
		if req.Schools == nil && req.SchoolID != nil && *req.SchoolID != nil {
			ownedSchools = []models.AdminSchoolRelation{{SchoolID: **req.SchoolID, CommissionRate: 0, IsOwner: true}}
		}
		if len(ownedSchools) == 0 {
			return response.BadRequest(ctx, "校区超级管理员至少需要绑定一个学校")
		}
		target.SchoolID = nil
		if err := s.validateSchoolCommissionCapacity(ctx, target.ID, ownedSchools); err != nil {
			return response.BadRequest(ctx, err.Error())
		}
	}
	if req.Status != nil {
		target.Status = *req.Status
	}
	if req.JoinDate != nil {
		if strings.TrimSpace(*req.JoinDate) == "" {
			target.JoinDate = nil
		} else {
			parsed, err := time.Parse("2006-01-02", *req.JoinDate)
			if err != nil {
				return response.BadRequest(ctx, "joinDate must use YYYY-MM-DD")
			}
			target.JoinDate = &parsed
		}
	}
	if req.Intro != nil {
		value := strings.TrimSpace(*req.Intro)
		target.Intro = &value
	}
	if req.ArticleURL != nil {
		value := strings.TrimSpace(*req.ArticleURL)
		target.ArticleURL = &value
	}
	if callerRole == models.AdminRoleSchoolSuperAdmin && id != callerID {
		allowed, err := s.canAccessSchool(ctx, target.SchoolID)
		if err != nil || (target.Role != models.AdminRoleSchoolAdmin && target.Role != models.AdminRoleEventManager) || !allowed {
			return response.Forbidden(ctx, "校区超级管理员只能编辑本校管理员")
		}
	}

	target.PasswordHash = ""
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return response.InternalError(ctx, "密码加密失败")
		}
		target.PasswordHash = string(hash)
		encrypted, encryptErr := encryptAdminCredential(req.Password)
		if encryptErr != nil {
			return response.InternalError(ctx, "管理员密码安全存储失败")
		}
		target.PasswordEncrypted = &encrypted
	}

	if callerRole == models.AdminRoleSuperAdmin && (originalRole == models.AdminRoleSchoolSuperAdmin || target.Role == models.AdminRoleSchoolSuperAdmin) {
		err = s.repo.AdminUser.UpdateWithSchools(ctx.Request().Context(), target, ownedSchools)
	} else {
		err = s.repo.AdminUser.Update(ctx.Request().Context(), target)
	}
	if err != nil {
		if errors.Is(err, repository.ErrSchoolAlreadyOwned) {
			return response.BadRequest(ctx, "所选学校已有校区超级管理员负责人")
		}
		if errors.Is(err, sql.ErrNoRows) {
			return response.NotFound(ctx, "管理员不存在")
		}
		return response.InternalError(ctx, "更新管理员失败")
	}

	updated, _ := s.repo.AdminUser.GetByID(ctx.Request().Context(), id)
	return response.Success(ctx, adminvo.NewAdminUserAccountVO(updated))
}

type updateAdminStatusRequest struct {
	Status int `json:"status"`
}

// UpdateAdminStatus handles PATCH /admin/admins/:id/status
func (s *AdminServer) UpdateAdminStatus(ctx echo.Context) error {
	callerRole := adminRole(ctx)
	callerID := currentAdminID(ctx)

	if callerRole == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, adminCenterForbiddenMessage)
	}

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid admin id")
	}
	var req updateAdminStatusRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	if req.Status != models.AdminUserStatusDisabled && req.Status != models.AdminUserStatusEnabled {
		return response.BadRequest(ctx, "status 只能为 0（禁用）或 1（启用）")
	}

	if id == callerID && req.Status == models.AdminUserStatusDisabled {
		return response.BadRequest(ctx, "不能禁用自己的账号")
	}

	target, err := s.repo.AdminUser.GetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "查询管理员失败")
	}
	if target == nil {
		return response.NotFound(ctx, "管理员不存在")
	}

	if !s.canEditAdminInScope(ctx, target) {
		return response.Forbidden(ctx, "权限不足")
	}

	if err := s.repo.AdminUser.UpdateStatus(ctx.Request().Context(), id, req.Status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return response.NotFound(ctx, "管理员不存在")
		}
		return response.InternalError(ctx, "更新状态失败")
	}

	return response.SuccessMessage(ctx, "操作成功")
}

// DeleteAdmin handles DELETE /admin/admins/:id
func (s *AdminServer) DeleteAdmin(ctx echo.Context) error {
	callerRole := adminRole(ctx)
	callerID := currentAdminID(ctx)

	if callerRole == models.AdminRoleSchoolAdmin {
		return response.Forbidden(ctx, adminCenterForbiddenMessage)
	}

	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid admin id")
	}
	if id == callerID {
		return response.BadRequest(ctx, "不能删除自己的管理员账号")
	}

	target, err := s.repo.AdminUser.GetByID(ctx.Request().Context(), id)
	if err != nil {
		return response.InternalError(ctx, "查询管理员失败")
	}
	if target == nil {
		return response.NotFound(ctx, "管理员不存在")
	}

	if !s.canEditAdminInScope(ctx, target) {
		return response.Forbidden(ctx, "权限不足")
	}

	if err := s.repo.AdminUser.Delete(ctx.Request().Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return response.NotFound(ctx, "管理员不存在")
		}
		return response.InternalError(ctx, "删除管理员失败")
	}

	return response.SuccessMessage(ctx, "操作成功")
}
