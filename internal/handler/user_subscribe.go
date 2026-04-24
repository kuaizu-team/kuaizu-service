package handler

import (
	"strings"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

// SyncUserSubscription handles POST /user/subscribe
func (s *Server) SyncUserSubscription(ctx echo.Context) error {
	var req api.SubscribeSyncRequest
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "参数解析失败")
	}

	userID := GetUserID(ctx)
	syncResults := make([]service.TemplateSyncResult, len(req.Templates))
	for i, t := range req.Templates {
		syncResults[i] = service.TemplateSyncResult{
			BizKey: t.BizKey,
			Result: string(t.Result),
		}
	}

	err := s.svc.Message.SyncSubscribeStatus(ctx.Request().Context(), userID, syncResults)
	if err != nil {
		return InternalError(ctx, "同步订阅状态失败")
	}

	return SuccessMessage(ctx, "同步成功")
}

// GetMsgTemplates handles GET /user/subscribe
func (s *Server) GetMsgTemplates(ctx echo.Context, params api.GetMsgTemplatesParams) error {
	bizKeys := strings.Split(params.BizKeys, ",")
	configs, err := s.svc.Message.GetMsgTemplatesByBizKeys(ctx.Request().Context(), bizKeys)
	if err != nil {
		return InternalError(ctx, "获取模板失败")
	}

	data := make([]api.MsgTemplateVO, len(configs))
	for i, c := range configs {
		data[i] = *c.ToVO()
	}

	return Success(ctx, data)
}
