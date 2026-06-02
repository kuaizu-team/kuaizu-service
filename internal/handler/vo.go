package handler

import (
	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// extendedUserVO embeds the generated api.UserVO and appends account-status fields.
// We cannot modify api.gen.go (generated), so we extend here in the handler layer.
type extendedUserVO struct {
	*api.UserVO
	UserStatus int     `json:"userStatus"`          // 0=正常, 1=封禁, 2=已毕业
	BanReason  *string `json:"banReason,omitempty"` // 封禁原因（仅 userStatus=1 时携带）
}

// toExtendedUserVO converts a raw models.User to extendedUserVO.
// Returns nil when u is nil.
func toExtendedUserVO(u *models.User) *extendedUserVO {
	if u == nil {
		return nil
	}
	return &extendedUserVO{
		UserVO:     u.ToVO(),
		UserStatus: u.UserStatus,
		BanReason:  u.BanReason,
	}
}

// loginResponse is the custom login response body that replaces the generated
// api.LoginResponse, allowing us to embed extendedUserVO instead of *api.UserVO.
type loginResponse struct {
	Token        *string         `json:"token,omitempty"`
	ExpiresIn    *int            `json:"expiresIn,omitempty"`
	IsFirstLogin *bool           `json:"isFirstLogin,omitempty"`
	IsNewUser    *bool           `json:"isNewUser,omitempty"`
	User         *extendedUserVO `json:"user,omitempty"`
}
