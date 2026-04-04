package handler

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/kuaizu-team/kuaizu-service/api"
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
		return Error(ctx, 4001, "微信登录失败: "+err.Error())
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

	// Return login response
	response := api.LoginResponse{
		Token:     result.Token,
		ExpiresIn: result.ExpiresIn,
		IsNewUser: result.IsNewUser,
		User:      result.User,
	}

	return Success(ctx, response)
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
		return Error(ctx, 4002, "手机号注册失败: "+err.Error())
	}

	response := api.LoginResponse{
		Token:     &result.Token,
		ExpiresIn: &result.ExpiresIn,
		IsNewUser: &result.IsNewUser,
		User:      result.User,
	}

	return Success(ctx, response)
}
