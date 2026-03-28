package service

import (
	"fmt"

	"github.com/trv3wood/kuaizu-server/internal/email"
	"github.com/trv3wood/kuaizu-server/internal/oss"
	"github.com/trv3wood/kuaizu-server/internal/repository"
	"github.com/trv3wood/kuaizu-server/internal/wechat"
)

// Dependencies holds external clients and shared integrations for service wiring.
type Dependencies struct {
	OSSClient      *oss.Client
	WechatClient   *wechat.Client
	PayClient      *wechat.PayClient
	PayInitError   error
	EmailService   *email.Service
	EmailInitError error
}

// NewDependencies builds shared service dependencies from environment-backed clients.
func NewDependencies(repo *repository.Repository) (*Dependencies, error) {
	ossClient, err := oss.NewClient()
	if err != nil {
		return nil, fmt.Errorf("init oss client: %w", err)
	}

	wxClient := wechat.NewClient()

	payConfig, payConfigErr := wechat.DefaultPayConfig()
	var payClient *wechat.PayClient
	var payErr error
	if payConfigErr != nil {
		payErr = payConfigErr
	} else {
		payClient, payErr = wechat.NewPayClient(payConfig)
	}

	emailService, emailErr := email.NewServiceFromEnv(
		repo.User,
		repo.Project,
		repo.EmailPromotion,
	)

	return &Dependencies{
		OSSClient:      ossClient,
		WechatClient:   wxClient,
		PayClient:      payClient,
		PayInitError:   payErr,
		EmailService:   emailService,
		EmailInitError: emailErr,
	}, nil
}
