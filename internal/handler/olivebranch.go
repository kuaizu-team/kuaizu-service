package handler

import (
	"github.com/labstack/echo/v4"
	"github.com/trv3wood/kuaizu-server/api"
	"github.com/trv3wood/kuaizu-server/internal/repository"
	"github.com/trv3wood/kuaizu-server/internal/service"
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
