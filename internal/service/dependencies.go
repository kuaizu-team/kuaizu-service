package service

import (
	"fmt"
	"log"

	"github.com/kuaizu-team/kuaizu-service/internal/messagecenter"
	"github.com/kuaizu-team/kuaizu-service/internal/oss"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
)

// Dependencies holds external clients and shared integrations for service wiring.
type Dependencies struct {
	OSSClient              *oss.Client
	WechatClient           *wechat.Client
	PayClient              *wechat.PayClient
	PayInitError           error
	MessageCenter          *messagecenter.Client
	MessageCenterInitError error
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

	messageCenter, messageCenterErr := messagecenter.NewClientFromEnv()
	if messageCenterErr != nil {
		log.Printf("[Dependencies] message center not configured: %v", messageCenterErr)
	} else {
		log.Printf("[Dependencies] message center configured, base_url=%s", messageCenter.BaseURL())
	}

	return &Dependencies{
		OSSClient:              ossClient,
		WechatClient:           wxClient,
		PayClient:              payClient,
		PayInitError:           payErr,
		MessageCenter:          messageCenter,
		MessageCenterInitError: messageCenterErr,
	}, nil
}
