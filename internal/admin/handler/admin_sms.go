package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/kuaizu-team/kuaizu-service/internal/messagecenter"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/labstack/echo/v4"
)

type adminSmsSendRequest struct {
	TemplateKey string                 `json:"template_key"`
	UserID      int                    `json:"user_id"`
	Variables   map[string]interface{} `json:"variables"`
}

func (s *AdminServer) SendAdminSms(ctx echo.Context) error {
	var req adminSmsSendRequest
	if err := ctx.Bind(&req); err != nil {
		return response.BadRequest(ctx, "invalid request body")
	}
	if err := s.ensureAdminCanAccessUser(ctx, req.UserID); err != nil {
		return err
	}

	resp, err := s.svc.AdminSms.Send(ctx.Request().Context(), messagecenter.AdminSmsSendRequest{
		TemplateKey: req.TemplateKey,
		UserID:      req.UserID,
		Variables:   req.Variables,
	})
	if err != nil {
		return mapServiceError(ctx, err)
	}
	if resp != nil && resp.Success && strings.EqualFold(strings.TrimSpace(req.TemplateKey), "INVITE_SUPER_ADMIN") {
		if err := s.svc.Invitation.ResetAfterInviteSent(ctx.Request().Context(), req.UserID); err != nil {
			return mapServiceError(ctx, err)
		}
	}

	return adminSmsSuccess(ctx, resp)
}

func (s *AdminServer) CountAdminSms(ctx echo.Context) error {
	userID, err := strconv.Atoi(ctx.QueryParam("user_id"))
	if err != nil {
		return response.BadRequest(ctx, "invalid user_id")
	}
	if err := s.ensureAdminCanAccessUser(ctx, userID); err != nil {
		return err
	}

	days := 30
	if v := ctx.QueryParam("days"); v != "" {
		days, err = strconv.Atoi(v)
		if err != nil {
			return response.BadRequest(ctx, "invalid days")
		}
	}

	resp, err := s.svc.AdminSms.Count(ctx.Request().Context(), userID, ctx.QueryParam("template_key"), days)
	if err != nil {
		return mapServiceError(ctx, err)
	}

	return adminSmsSuccess(ctx, resp)
}

func (s *AdminServer) ensureAdminCanAccessUser(ctx echo.Context, userID int) error {
	if userID <= 0 {
		return response.BadRequest(ctx, "invalid user_id")
	}
	if sid := adminSchoolID(ctx); sid != nil {
		user, err := s.svc.User.GetUser(ctx.Request().Context(), userID)
		if err != nil {
			return mapServiceError(ctx, err)
		}
		if user == nil || user.SchoolID == nil || *user.SchoolID != *sid {
			return response.Forbidden(ctx, "permission denied")
		}
	}
	return nil
}

func adminSmsSuccess(ctx echo.Context, data interface{}) error {
	return ctx.JSON(http.StatusOK, response.Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    data,
	})
}
