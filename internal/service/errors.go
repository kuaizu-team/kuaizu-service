package service

import "fmt"

// ErrorCode represents the type of service error for HTTP status mapping.
type ErrorCode int

const (
	ErrCodeBadRequest ErrorCode = 400
	ErrCodeForbidden  ErrorCode = 403
	ErrCodeNotFound   ErrorCode = 404
	ErrCodeInternal   ErrorCode = 500

	// ErrCodeTooManyRequests signals a sliding-window rate limit violation.
	// The value 42901 is sent as the business code in the response body;
	// the HTTP status will be 429 Too Many Requests.
	ErrCodeTooManyRequests ErrorCode = 42901
)

// ServiceError is a business-level error returned by the service layer.
// Handler maps ErrorCode to HTTP status codes.
type ServiceError struct {
	Code    ErrorCode
	Message string
}

func (e *ServiceError) Error() string {
	return fmt.Sprintf("service error %d: %s", e.Code, e.Message)
}

func ErrBadRequest(msg string) *ServiceError {
	return &ServiceError{Code: ErrCodeBadRequest, Message: msg}
}

func ErrNotFound(msg string) *ServiceError {
	return &ServiceError{Code: ErrCodeNotFound, Message: msg}
}

func ErrForbidden(msg string) *ServiceError {
	return &ServiceError{Code: ErrCodeForbidden, Message: msg}
}

func ErrInternal(msg string) *ServiceError {
	return &ServiceError{Code: ErrCodeInternal, Message: msg}
}

func ErrTooManyRequests(msg string) *ServiceError {
	return &ServiceError{Code: ErrCodeTooManyRequests, Message: msg}
}
