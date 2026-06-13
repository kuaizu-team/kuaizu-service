package service

import (
	"context"
	"fmt"
	"log"

	"github.com/kuaizu-team/kuaizu-service/internal/auth"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
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
	IsFirstLogin      *bool
	IsNewUser         *bool
	User              *models.User // raw user; handler builds the VO to include userStatus/banReason
}

// WechatPrecheckResult represents a side-effect-free WeChat auth precheck result.
type WechatPrecheckResult struct {
	Registered        bool
	NeedsPhoneBinding bool
	RegisterToken     *string
	ExpiresIn         *int
}

// PrecheckWechatAuth checks whether a WeChat user can log in without issuing a login token.
func (s *AuthService) PrecheckWechatAuth(ctx context.Context, code string) (*WechatPrecheckResult, error) {
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}

	wxResp, err := s.wxClient.Code2Session(code)
	if err != nil {
		return nil, fmt.Errorf("wechat code2session failed: %w", err)
	}

	user, err := s.repo.User.GetByOpenID(ctx, wxResp.OpenID)
	if err != nil {
		return nil, fmt.Errorf("get user by openid failed: %w", err)
	}

	if user == nil || user.Phone == nil {
		registerConfig := auth.RegisterConfig()
		registerToken, expiresIn, err := auth.GenerateRegisterToken(registerConfig, wxResp.OpenID)
		if err != nil {
			return nil, fmt.Errorf("generate register token failed: %w", err)
		}

		return &WechatPrecheckResult{
			Registered:        user != nil,
			NeedsPhoneBinding: true,
			RegisterToken:     &registerToken,
			ExpiresIn:         &expiresIn,
		}, nil
	}

	return &WechatPrecheckResult{
		Registered:        true,
		NeedsPhoneBinding: false,
	}, nil
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

	_ = s.repo.User.TouchLastActiveDate(ctx, user.ID)
	if refreshedUser, err := s.repo.User.GetByID(ctx, user.ID); err == nil && refreshedUser != nil {
		user = refreshedUser
	}

	isNewUser := false
	return &LoginWithWechatResult{
		NeedsPhoneBinding: false,
		Token:             &token,
		ExpiresIn:         &expiresIn,
		IsFirstLogin:      &isNewUser,
		IsNewUser:         &isNewUser,
		User:              user,
	}, nil
}

// RegisterWithPhoneResult represents the result of phone registration
type RegisterWithPhoneResult struct {
	Token        string
	ExpiresIn    int
	IsFirstLogin bool
	IsNewUser    bool
	User         *models.User // raw user; handler builds the VO to include userStatus/banReason
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

	var isFirstLogin bool
	var isNewUser bool
	if user == nil {
		// Create new user
		isFirstLogin = true
		isNewUser = true
		user, err = s.repo.User.CreateWithPhone(ctx, claims.OpenID, phone)
		if err != nil {
			return nil, fmt.Errorf("create user failed: %w", err)
		}
	} else {
		// Update phone if not set
		isNewUser = false
		if user.Phone == nil || *user.Phone == "" {
			isFirstLogin = true
			if err := s.repo.User.UpdatePhone(ctx, user.ID, phone); err != nil {
				return nil, fmt.Errorf("update phone failed: %w", err)
			}
			user.Phone = &phone
		}
	}

	if err := s.attachRegisterInvitations(ctx, user.ID, phone); err != nil {
		log.Printf("[AuthService.RegisterWithPhone] attach register invitations failed, user_id=%d phone=%s: %v", user.ID, phone, err)
	}

	// Generate JWT token
	jwtConfig := auth.DefaultConfig()
	token, expiresIn, err := auth.GenerateToken(jwtConfig, user.ID, claims.OpenID)
	if err != nil {
		return nil, fmt.Errorf("generate token failed: %w", err)
	}

	_ = s.repo.User.TouchLastActiveDate(ctx, user.ID)
	if refreshedUser, err := s.repo.User.GetByID(ctx, user.ID); err == nil && refreshedUser != nil {
		user = refreshedUser
	}

	return &RegisterWithPhoneResult{
		Token:        token,
		ExpiresIn:    expiresIn,
		IsFirstLogin: isFirstLogin,
		IsNewUser:    isNewUser,
		User:         user,
	}, nil
}

func (s *AuthService) attachRegisterInvitations(ctx context.Context, userID int, phone string) error {
	if userID <= 0 || phone == "" {
		return nil
	}
	return s.repo.InvitationRecord.AttachPendingJoinsByPhone(ctx, userID, phone)
}
