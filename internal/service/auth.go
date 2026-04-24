package service

import (
	"context"
	"fmt"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/auth"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
)

type AuthService struct {
	repo     *repository.Repository
	wxClient *wechat.Client
}

func NewAuthService(repo *repository.Repository, wxClient *wechat.Client) *AuthService {
	return &AuthService{
		repo:     repo,
		wxClient: wxClient,
	}
}

// LoginWithWechatResult represents the result of WeChat login
type LoginWithWechatResult struct {
	NeedsPhoneBinding bool
	RegisterToken     *string
	ExpiresIn         *int
	Token             *string
	IsNewUser         *bool
	User              *api.UserVO
}

// LoginWithWechat handles WeChat login logic
func (s *AuthService) LoginWithWechat(ctx context.Context, code string) (*LoginWithWechatResult, error) {
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}

	// Call WeChat API to get openid
	wxResp, err := s.wxClient.Code2Session(code)
	if err != nil {
		return nil, fmt.Errorf("wechat code2session failed: %w", err)
	}

	// Check if user exists
	user, err := s.repo.User.GetByOpenID(ctx, wxResp.OpenID)
	if err != nil {
		return nil, fmt.Errorf("get user by openid failed: %w", err)
	}

	// If user doesn't exist or phone is null, return register token
	if user == nil || user.Phone == nil {
		registerConfig := auth.RegisterConfig()
		registerToken, expiresIn, err := auth.GenerateRegisterToken(registerConfig, wxResp.OpenID)
		if err != nil {
			return nil, fmt.Errorf("generate register token failed: %w", err)
		}

		return &LoginWithWechatResult{
			NeedsPhoneBinding: true,
			RegisterToken:     &registerToken,
			ExpiresIn:         &expiresIn,
		}, nil
	}

	// Generate JWT token
	jwtConfig := auth.DefaultConfig()
	token, expiresIn, err := auth.GenerateToken(jwtConfig, user.ID, wxResp.OpenID)
	if err != nil {
		return nil, fmt.Errorf("generate token failed: %w", err)
	}

	isNewUser := false
	return &LoginWithWechatResult{
		NeedsPhoneBinding: false,
		Token:             &token,
		ExpiresIn:         &expiresIn,
		IsNewUser:         &isNewUser,
		User:              user.ToVO(),
	}, nil
}

// RegisterWithPhoneResult represents the result of phone registration
type RegisterWithPhoneResult struct {
	Token     string
	ExpiresIn int
	IsNewUser bool
	User      *api.UserVO
}

// RegisterWithPhone handles phone registration logic
func (s *AuthService) RegisterWithPhone(ctx context.Context, registerToken, phoneCode string) (*RegisterWithPhoneResult, error) {
	if registerToken == "" {
		return nil, fmt.Errorf("registerToken is required")
	}
	if phoneCode == "" {
		return nil, fmt.Errorf("phoneCode is required")
	}

	// Parse register token
	registerConfig := auth.RegisterConfig()
	claims, err := auth.ParseRegisterToken(registerConfig, registerToken)
	if err != nil {
		return nil, fmt.Errorf("invalid or expired registerToken: %w", err)
	}

	// Get phone number from WeChat
	phone, err := s.wxClient.GetPhoneNumber(phoneCode)
	if err != nil {
		return nil, fmt.Errorf("get phone number failed: %w", err)
	}

	// Check if user exists
	user, err := s.repo.User.GetByOpenID(ctx, claims.OpenID)
	if err != nil {
		return nil, fmt.Errorf("get user by openid failed: %w", err)
	}

	var isNewUser bool
	if user == nil {
		// Create new user
		isNewUser = true
		user, err = s.repo.User.CreateWithPhone(ctx, claims.OpenID, phone)
		if err != nil {
			return nil, fmt.Errorf("create user failed: %w", err)
		}
	} else {
		// Update phone if not set
		isNewUser = false
		if user.Phone == nil || *user.Phone == "" {
			if err := s.repo.User.UpdatePhone(ctx, user.ID, phone); err != nil {
				return nil, fmt.Errorf("update phone failed: %w", err)
			}
			user.Phone = &phone
		}
	}

	// Generate JWT token
	jwtConfig := auth.DefaultConfig()
	token, expiresIn, err := auth.GenerateToken(jwtConfig, user.ID, claims.OpenID)
	if err != nil {
		return nil, fmt.Errorf("generate token failed: %w", err)
	}

	return &RegisterWithPhoneResult{
		Token:     token,
		ExpiresIn: expiresIn,
		IsNewUser: isNewUser,
		User:      user.ToVO(),
	}, nil
}
