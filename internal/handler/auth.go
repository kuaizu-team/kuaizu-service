package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
)

// LoginWithWechat handles POST /auth/login/wechat
// This endpoint handles login:
// - If user exists: returns token
// - If user doesn't exist: returns registerToken for phone binding
func (s *Server) LoginWithWechat(ctx echo.Context) error {
	// Bind request body
	var req api.LoginWithWechatJSONRequestBody
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	if req.Code == "" {
		return BadRequest(ctx, "微信登录code不能为空")
	}

	// Call service layer
	result, err := s.svc.Auth.LoginWithWechat(ctx.Request().Context(), req.Code)
	if err != nil {
		log.Printf("LoginWithWechat error: %v", err)
		return Error(ctx, 4001, "微信登录失败")
	}

	// If user needs phone binding
	if result.NeedsPhoneBinding {
		data := api.RegisterTokenResponse{
			RegisterToken: result.RegisterToken,
			ExpiresIn:     result.ExpiresIn,
		}

		return ctx.JSON(http.StatusAccepted, Response{
			Code:    1001,
			Message: "需要绑定手机号",
			Data:    data,
		})
	}

	// Return login response — use custom struct to carry userStatus/banReason
	return Success(ctx, loginResponse{
		Token:        result.Token,
		ExpiresIn:    result.ExpiresIn,
		IsFirstLogin: result.IsFirstLogin,
		IsNewUser:    result.IsNewUser,
		User:         toExtendedUserVO(result.User),
	})
}

// PrecheckWechatAuth handles POST /auth/precheck/wechat.
// It checks registration state without issuing a login token or touching login activity.
func (s *Server) PrecheckWechatAuth(ctx echo.Context) error {
	var req api.PrecheckWechatAuthJSONRequestBody
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	if req.Code == "" {
		return BadRequest(ctx, "微信登录code不能为空")
	}

	result, err := s.svc.Auth.PrecheckWechatAuth(ctx.Request().Context(), req.Code)
	if err != nil {
		log.Printf("PrecheckWechatAuth error: %v", err)
		return Error(ctx, 4001, "微信登录预检查失败")
	}

	return Success(ctx, api.WechatPrecheckResponse{
		Registered:        result.Registered,
		NeedsPhoneBinding: result.NeedsPhoneBinding,
		RegisterToken:     result.RegisterToken,
		ExpiresIn:         result.ExpiresIn,
	})
}

// RegisterWithPhone handles POST /auth/register/phone
// This endpoint completes registration by binding phone number and issuing a token
func (s *Server) RegisterWithPhone(ctx echo.Context) error {
	var req api.RegisterWithPhoneJSONRequestBody
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "请求参数错误")
	}

	if req.RegisterToken == "" {
		return BadRequest(ctx, "registerToken不能为空")
	}
	if req.PhoneCode == "" {
		return BadRequest(ctx, "phoneCode不能为空")
	}

	// Call service layer
	result, err := s.svc.Auth.RegisterWithPhone(ctx.Request().Context(), req.RegisterToken, req.PhoneCode)
	if err != nil {
		log.Printf("RegisterWithPhone error: %v", err)
		if errors.Is(err, repository.ErrUserPhoneConflict) {
			return Error(ctx, 4002, "该手机号已绑定其他账号")
		}
		return Error(ctx, 4002, "手机号注册失败")
	}

	return Success(ctx, loginResponse{
		Token:        &result.Token,
		ExpiresIn:    &result.ExpiresIn,
		IsFirstLogin: &result.IsFirstLogin,
		IsNewUser:    &result.IsNewUser,
		User:         toExtendedUserVO(result.User),
	})
}
