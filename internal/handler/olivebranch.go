package handler

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

// GetMyReceivedOliveBranches handles GET /users/me/olive-branches
func (s *Server) GetMyReceivedOliveBranches(ctx echo.Context, params api.GetMyReceivedOliveBranchesParams) error {
	userID := GetUserID(ctx)

	listParams := repository.OliveBranchListParams{
		ReceiverID: userID,
		Page:       1,
		Size:       10,
	}

	if params.Page != nil {
		listParams.Page = *params.Page
	}
	if params.Size != nil {
		listParams.Size = *params.Size
	}
	if listParams.Page < 1 {
		listParams.Page = 1
	}
	if listParams.Size < 1 || listParams.Size > 100 {
		listParams.Size = 10
	}

	if params.Status != nil {
		status := int(*params.Status)
		listParams.Status = &status
	}

	records, total, err := s.repo.OliveBranch.ListByReceiverID(ctx.Request().Context(), listParams)
	if err != nil {
		return InternalError(ctx, "获取橄榄枝列表失败")
	}

	list := make([]api.OliveBranchVO, len(records))
	for i, ob := range records {
		list[i] = *ob.ToVO()
	}

	totalPages := int((total + int64(listParams.Size) - 1) / int64(listParams.Size))
	pageInfo := api.PageInfo{
		Page:       &listParams.Page,
		Size:       &listParams.Size,
		Total:      &total,
		TotalPages: &totalPages,
	}

	return Success(ctx, api.OliveBranchPageResponse{
		List:     &list,
		PageInfo: &pageInfo,
	})
}

// SendOliveBranch handles POST /olive-branches
func (s *Server) SendOliveBranch(ctx echo.Context) error {
	userID := GetUserID(ctx)

	var req api.SendOliveBranchJSONRequestBody
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	ob, err := s.svc.OliveBranch.SendOliveBranch(ctx.Request().Context(), userID, service.SendRequest{
		ReceiverID:       req.ReceiverId,
		RelatedProjectID: req.RelatedProjectId,
	})
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, ob.ToVO())
}

// HandleOliveBranch handles PATCH /olive-branches/{id}
func (s *Server) HandleOliveBranch(ctx echo.Context, id int) error {
	userID := GetUserID(ctx)

	var req api.HandleOliveBranchJSONBody
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	ob, err := s.svc.OliveBranch.HandleOliveBranch(ctx.Request().Context(), userID, id, string(req.Action))
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, ob.ToVO())
}

// GetMyOliveBranchQuota handles GET /users/me/olive-branch-quota
func (s *Server) GetMyOliveBranchQuota(ctx echo.Context) error {
	userID := GetUserID(ctx)

	user, err := s.repo.User.GetByID(ctx.Request().Context(), userID)
	if err != nil {
		return InternalError(ctx, "获取用户信息失败")
	}
	if user == nil {
		return NotFound(ctx, "用户不存在")
	}

	const dailyFreeQuota = 5

	freeBranchUsedToday := 0
	if user.FreeBranchUsedToday != nil {
		today := time.Now().Truncate(24 * time.Hour)
		if user.LastActiveDate != nil && !user.LastActiveDate.Truncate(24*time.Hour).Before(today) {
			freeBranchUsedToday = *user.FreeBranchUsedToday
		}
	}

	paidBalance := 0
	if user.OliveBranchCount != nil {
		paidBalance = *user.OliveBranchCount
	}

	freeRemaining := dailyFreeQuota - freeBranchUsedToday
	totalRemaining := freeRemaining + paidBalance

	dq := dailyFreeQuota
	fr := freeRemaining
	tr := totalRemaining

	return Success(ctx, api.OliveBranchQuotaVO{
		DailyFreeQuota:      &dq,
		FreeBranchUsedToday: &freeBranchUsedToday,
		FreeRemaining:       &fr,
		PaidBalance:         &paidBalance,
		TotalRemaining:      &tr,
	})
}

// GetMySentOliveBranches handles GET /users/me/sent-olive-branches
func (s *Server) GetMySentOliveBranches(ctx echo.Context, params api.GetMySentOliveBranchesParams) error {
	userID := GetUserID(ctx)

	listParams := repository.OliveBranchListParams{
		SenderID: userID,
		Page:     1,
		Size:     10,
	}

	if params.Page != nil {
		listParams.Page = *params.Page
	}
	if params.Size != nil {
		listParams.Size = *params.Size
	}
	if listParams.Page < 1 {
		listParams.Page = 1
	}
	if listParams.Size < 1 || listParams.Size > 100 {
		listParams.Size = 10
	}

	if params.Status != nil {
		status := int(*params.Status)
		listParams.Status = &status
	}

	records, total, err := s.repo.OliveBranch.ListBySenderID(ctx.Request().Context(), listParams)
	if err != nil {
		return InternalError(ctx, "获取橄榄枝列表失败")
	}

	list := make([]api.OliveBranchVO, len(records))
	for i, ob := range records {
		list[i] = *ob.ToVO()
	}

	totalPages := int((total + int64(listParams.Size) - 1) / int64(listParams.Size))
	pageInfo := api.PageInfo{
		Page:       &listParams.Page,
		Size:       &listParams.Size,
		Total:      &total,
		TotalPages: &totalPages,
	}

	return Success(ctx, api.OliveBranchPageResponse{
		List:     &list,
		PageInfo: &pageInfo,
	})
}
