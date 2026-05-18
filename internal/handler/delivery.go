package handler

import "github.com/labstack/echo/v4"

// deliveryQuotaResponse is the response body for GET /api/v2/users/me/delivery/quota.
type deliveryQuotaResponse struct {
	UsedCount int    `json:"usedCount"`
	Limit     int    `json:"limit"`
	Remaining int    `json:"remaining"`
	ResetAt   *int64 `json:"resetAt,omitempty"` // Unix timestamp of next slot expiry; nil when window is empty
}

// GetDeliveryQuota returns the caller's current sliding-window delivery quota.
//
// GET /api/v2/users/me/delivery/quota
func (s *Server) GetDeliveryQuota(ctx echo.Context) error {
	userID := GetUserID(ctx)

	used, limit, resetAt, err := s.svc.Project.GetDeliveryQuota(ctx.Request().Context(), userID)
	if err != nil {
		return InternalError(ctx, "获取投递配额失败")
	}

	remaining := limit - used
	if remaining < 0 {
		remaining = 0
	}

	return Success(ctx, deliveryQuotaResponse{
		UsedCount: used,
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   resetAt,
	})
}
