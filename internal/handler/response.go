package handler

import (
	"errors"

	"github.com/labstack/echo/v4"
	"github.com/kuaizu-team/kuaizu-service/internal/response"
	"github.com/kuaizu-team/kuaizu-service/internal/service"
)

// Re-export response types and helpers from shared package
// so existing handler code continues to work without changes.

type Response = response.Response

func Success(ctx echo.Context, data interface{}) error { return response.Success(ctx, data) }
func SuccessMessage(ctx echo.Context, message string) error {
	return response.SuccessMessage(ctx, message)
}
func Error(ctx echo.Context, code int, message string) error {
	return response.Error(ctx, code, message)
}
func BadRequest(ctx echo.Context, message string) error   { return response.BadRequest(ctx, message) }
func InvalidParams(ctx echo.Context, err error) error     { return response.InvalidParams(ctx, err) }
func Unauthorized(ctx echo.Context, message string) error { return response.Unauthorized(ctx, message) }
func Forbidden(ctx echo.Context, message string) error    { return response.Forbidden(ctx, message) }
func NotFound(ctx echo.Context, message string) error     { return response.NotFound(ctx, message) }
func InternalError(ctx echo.Context, message string) error {
	return response.InternalError(ctx, message)
}
func NotImplemented(ctx echo.Context) error { return response.NotImplemented(ctx) }

// mapServiceError maps a service.ServiceError to the appropriate HTTP error response.
// For non-ServiceError errors, it falls back to InternalError.
func mapServiceError(ctx echo.Context, err error) error {
	var svcErr *service.ServiceError
	if errors.As(err, &svcErr) {
		switch svcErr.Code {
		case service.ErrCodeBadRequest:
			return BadRequest(ctx, svcErr.Message)
		case service.ErrCodeNotFound:
			return NotFound(ctx, svcErr.Message)
		case service.ErrCodeForbidden:
			return Forbidden(ctx, svcErr.Message)
		default:
			// For custom business codes (like 4002), use Error()
			return Error(ctx, int(svcErr.Code), svcErr.Message)
		}
	}
	return InternalError(ctx, err.Error())
}
