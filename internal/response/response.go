package response

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Response is the standard API response structure
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Success returns a successful response with data
func Success(ctx echo.Context, data interface{}) error {
	return ctx.JSON(http.StatusOK, Response{
		Code:    200,
		Message: "操作成功",
		Data:    data,
	})
}

// SuccessMessage returns a successful response with custom message
func SuccessMessage(ctx echo.Context, message string) error {
	return ctx.JSON(http.StatusOK, Response{
		Code:    200,
		Message: message,
	})
}

// Error returns an error response
func Error(ctx echo.Context, code int, message string) error {
	httpStatus := http.StatusOK // Always return 200 with business error code
	if code >= 500 {
		httpStatus = http.StatusInternalServerError
	}
	return ctx.JSON(httpStatus, Response{
		Code:    code,
		Message: message,
	})
}

// BadRequest returns a 400 bad request error
func BadRequest(ctx echo.Context, message string) error {
	return ctx.JSON(http.StatusBadRequest, Response{
		Code:    400,
		Message: message,
	})
}

// InvalidParams returns a 400 bad request error with error details
func InvalidParams(ctx echo.Context, err error) error {
	return ctx.JSON(http.StatusBadRequest, Response{
		Code:    400,
		Message: "Invalid parameters: " + err.Error(),
	})
}

// Unauthorized returns a 401 unauthorized error
func Unauthorized(ctx echo.Context, message string) error {
	return ctx.JSON(http.StatusUnauthorized, Response{
		Code:    401,
		Message: message,
	})
}

// Forbidden returns a 403 forbidden error
func Forbidden(ctx echo.Context, message string) error {
	return ctx.JSON(http.StatusForbidden, Response{
		Code:    403,
		Message: message,
	})
}

// NotFound returns a 404 not found error
func NotFound(ctx echo.Context, message string) error {
	return ctx.JSON(http.StatusNotFound, Response{
		Code:    404,
		Message: message,
	})
}

// InternalError returns a 500 internal server error
func InternalError(ctx echo.Context, message string) error {
	return ctx.JSON(http.StatusInternalServerError, Response{
		Code:    500,
		Message: message,
	})
}

// NotImplemented returns a 501 not implemented error
func NotImplemented(ctx echo.Context) error {
	return ctx.JSON(http.StatusNotImplemented, Response{
		Code:    501,
		Message: "接口尚未实现",
	})
}

// TooManyRequests returns a 429 Too Many Requests response.
// code is the business-level error code sent in the response body (e.g. 42901).
func TooManyRequests(ctx echo.Context, code int, message string) error {
	return ctx.JSON(http.StatusTooManyRequests, Response{
		Code:    code,
		Message: message,
	})
}
