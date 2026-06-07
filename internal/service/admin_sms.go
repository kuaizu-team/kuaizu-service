package service

import (
	"context"
	"log"
	"strings"

	"github.com/kuaizu-team/kuaizu-service/internal/messagecenter"
)

var supportedAdminSmsTemplateKeys = map[string]struct{}{
	"URGE_PROCESS":       {},
	"INVITE_SUPER_ADMIN": {},
}

type adminSmsSender interface {
	SendAdminSms(ctx context.Context, req messagecenter.AdminSmsSendRequest) (*messagecenter.AdminSmsSendResponse, error)
	CountAdminSms(ctx context.Context, userID int, templateKey string, days int) (*messagecenter.AdminSmsSendCountResponse, error)
}

type AdminSmsService struct {
	messageCenter        adminSmsSender
	messageCenterInitErr error
	messageCenterFactory func() (*messagecenter.Client, error)
}

func NewAdminSmsService(messageCenter *messagecenter.Client, messageCenterInitErr error) *AdminSmsService {
	svc := &AdminSmsService{
		messageCenterInitErr: messageCenterInitErr,
		messageCenterFactory: messagecenter.NewClientFromEnv,
	}
	if messageCenter != nil {
		svc.messageCenter = messageCenter
	}
	return svc
}

func (s *AdminSmsService) Send(ctx context.Context, req messagecenter.AdminSmsSendRequest) (*messagecenter.AdminSmsSendResponse, error) {
	templateKey, err := normalizeAdminSmsTemplateKey(req.TemplateKey)
	if err != nil {
		return nil, err
	}
	if req.UserID <= 0 {
		return nil, ErrBadRequest("user_id is required")
	}
	req.TemplateKey = templateKey

	client, initErr := s.resolveMessageCenter()
	if initErr != nil {
		log.Printf("[AdminSmsService] message center unavailable for send, user_id=%d template_key=%s: %v", req.UserID, templateKey, initErr)
		return nil, ErrInternal("message center is not configured: " + initErr.Error())
	}
	if client == nil {
		return nil, ErrInternal("message center client is nil")
	}

	resp, err := client.SendAdminSms(ctx, req)
	if err != nil {
		log.Printf("[AdminSmsService] send admin sms failed, user_id=%d template_key=%s: %v", req.UserID, templateKey, err)
		return nil, ErrInternal("send admin sms failed: " + err.Error())
	}
	return resp, nil
}

func (s *AdminSmsService) Count(ctx context.Context, userID int, templateKey string, days int) (*messagecenter.AdminSmsSendCountResponse, error) {
	normalizedTemplateKey, err := normalizeAdminSmsTemplateKey(templateKey)
	if err != nil {
		return nil, err
	}
	if userID <= 0 {
		return nil, ErrBadRequest("user_id is required")
	}
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		return nil, ErrBadRequest("days must be less than or equal to 365")
	}

	client, initErr := s.resolveMessageCenter()
	if initErr != nil {
		log.Printf("[AdminSmsService] message center unavailable for count, user_id=%d template_key=%s days=%d: %v", userID, normalizedTemplateKey, days, initErr)
		return nil, ErrInternal("message center is not configured: " + initErr.Error())
	}
	if client == nil {
		return nil, ErrInternal("message center client is nil")
	}

	resp, err := client.CountAdminSms(ctx, userID, normalizedTemplateKey, days)
	if err != nil {
		log.Printf("[AdminSmsService] count admin sms failed, user_id=%d template_key=%s days=%d: %v", userID, normalizedTemplateKey, days, err)
		return nil, ErrInternal("count admin sms failed: " + err.Error())
	}
	return resp, nil
}

func (s *AdminSmsService) resolveMessageCenter() (adminSmsSender, error) {
	if s == nil {
		return nil, ErrInternal("admin sms service is nil")
	}
	if s.messageCenter != nil && s.messageCenterInitErr == nil {
		return s.messageCenter, nil
	}
	factory := s.messageCenterFactory
	if factory == nil {
		factory = messagecenter.NewClientFromEnv
	}
	client, err := factory()
	if err != nil {
		s.messageCenterInitErr = err
		return s.messageCenter, s.messageCenterInitErr
	}
	s.messageCenter = client
	s.messageCenterInitErr = nil
	log.Printf("[AdminSmsService] message center configured after lazy reload, base_url=%s", client.BaseURL())
	return s.messageCenter, nil
}

func normalizeAdminSmsTemplateKey(templateKey string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(templateKey))
	if normalized == "" {
		return "", ErrBadRequest("template_key is required")
	}
	if _, ok := supportedAdminSmsTemplateKeys[normalized]; !ok {
		return "", ErrBadRequest("unsupported admin sms template_key: " + templateKey)
	}
	return normalized, nil
}
