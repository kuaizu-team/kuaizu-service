package handler

import (
	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

func (s *Server) ListMemberTimeline(ctx echo.Context, id int, params api.ListMemberTimelineParams) error {
	memberID := 0
	if params.MemberId != nil {
		memberID = *params.MemberId
	}
	result, err := s.svc.Project.ListMemberTimeline(ctx.Request().Context(), id, GetUserID(ctx), memberID)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, result)
}

func (s *Server) CreateMemberTimeline(ctx echo.Context, id int) error {
	return s.saveMemberTimeline(ctx, id, 0)
}

func (s *Server) UpdateMemberTimeline(ctx echo.Context, id int, nodeId int) error {
	if nodeId <= 0 {
		return BadRequest(ctx, "节点ID无效")
	}
	return s.saveMemberTimeline(ctx, id, nodeId)
}

func (s *Server) saveMemberTimeline(ctx echo.Context, projectID, nodeID int) error {
	var input service.MemberTimelineInput
	if err := ctx.Bind(&input); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}
	id, err := s.svc.Project.SaveMemberTimeline(ctx.Request().Context(), projectID, GetUserID(ctx), nodeID, input)
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, map[string]int{"id": id})
}

func (s *Server) DeleteMemberTimeline(ctx echo.Context, id int, nodeId int) error {
	if err := s.svc.Project.DeleteMemberTimeline(ctx.Request().Context(), id, GetUserID(ctx), nodeId); err != nil {
		return mapServiceError(ctx, err)
	}
	return SuccessMessage(ctx, "已删除时间节点")
}
