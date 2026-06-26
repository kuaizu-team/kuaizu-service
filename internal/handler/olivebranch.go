package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"strconv"
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
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

	role := ""
	if req.Role != nil {
		role = *req.Role
	}
	ob, err := s.svc.OliveBranch.HandleOliveBranch(ctx.Request().Context(), userID, id, string(req.Action), role)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, ob.ToVO())
}

// ResendOliveBranch handles POST /olive-branches/{id}/resend.
func (s *Server) ResendOliveBranch(ctx echo.Context) error {
	userID := GetUserID(ctx)
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		return BadRequest(ctx, "橄榄枝ID不正确")
	}

	ob, err := s.svc.OliveBranch.ResendOliveBranch(ctx.Request().Context(), userID, id)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return Success(ctx, ob.ToVO())
}

// GetMyOliveBranchQuota handles GET /users/me/olive-branch-quota
func (s *Server) GetMyOliveBranchQuota(ctx echo.Context) error {
	userID := GetUserID(ctx)

	if err := s.repo.User.ResetDailyFreeBranchQuotaIfNeeded(ctx.Request().Context(), userID); err != nil {
		return InternalError(ctx, "更新额度失败")
	}

	user, err := s.repo.User.GetByID(ctx.Request().Context(), userID)
	if err != nil {
		return InternalError(ctx, "获取用户信息失败")
	}
	if user == nil {
		return NotFound(ctx, "用户不存在")
	}

	quota := models.CalculateOliveBranchQuota(user, time.Now())
	dq := quota.DailyFreeQuota
	freeBranchUsedToday := quota.FreeBranchUsedToday
	fr := quota.FreeRemaining
	paidBalance := quota.PaidBalance
	tr := quota.TotalRemaining

	return Success(ctx, api.OliveBranchQuotaVO{
		DailyFreeQuota:      &dq,
		FreeBranchUsedToday: &freeBranchUsedToday,
		FreeRemaining:       &fr,
		PaidBalance:         &paidBalance,
		TotalRemaining:      &tr,
	})
}

// GetOliveBranchBadge handles GET /olive-branches/badge
// Returns receivedPendingCount (status=0 received) and sentUnreadCount (sent after last-viewed timestamp).
func (s *Server) GetOliveBranchBadge(ctx echo.Context) error {
	userID := GetUserID(ctx)

	counts, err := s.repo.OliveBranch.GetBadgeCounts(ctx.Request().Context(), userID)
	if err != nil {
		return InternalError(ctx, "获取橄榄枝徽章数据失败")
	}

	return Success(ctx, map[string]int{
		"receivedPendingCount": counts.ReceivedPendingCount,
		"sentUnreadCount":      counts.SentUnreadCount,
	})
}

// MarkSentOliveBranchRead handles POST /olive-branches/badge/mark-sent-read
// Updates sent_olive_viewed_at to NOW() so that sentUnreadCount resets to 0.
func (s *Server) MarkSentOliveBranchRead(ctx echo.Context) error {
	userID := GetUserID(ctx)

	if err := s.repo.User.UpdateSentOliveViewedAt(ctx.Request().Context(), userID); err != nil {
		return InternalError(ctx, "标记已读失败")
	}

	return Success(ctx, nil)
}

// MarkReceiverOliveBranchRead handles POST /olive-branches/received/mark-read
// Called by the receiver to mark their received olive branches as read.
func (s *Server) MarkReceiverOliveBranchRead(ctx echo.Context) error {
	userID := GetUserID(ctx)

	var req struct {
		Ids []int `json:"ids"`
	}
	body, err := io.ReadAll(ctx.Request().Body)
	if err != nil {
		return BadRequest(ctx, "请求参数错误")
	}
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return BadRequest(ctx, "请求参数错误")
		}
	}

	log.Printf("mark receiver olive branch read request: userID=%d ids=%v", userID, req.Ids)
	if err := s.repo.OliveBranch.MarkReceiverRead(ctx.Request().Context(), userID, req.Ids); err != nil {
		return InternalError(ctx, "标记已读失败")
	}

	return Success(ctx, nil)
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
