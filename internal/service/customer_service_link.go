package service

import (
	"context"
	"sync"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
)

const (
	customerServicePagePath = "/pages/contact-servicePPL/contact-servicePPL"
	urlLinkValidity         = 30 * 24 * time.Hour
	urlLinkRefreshAfter     = 23 * 24 * time.Hour
)

type urlLinkGenerator interface {
	GenerateURLLink(ctx context.Context, input wechat.URLLinkRequest) (string, error)
}

// CustomerServiceLinkService keeps the public email URL stable while rotating
// the underlying WeChat URL Link before its 30-day expiration.
type CustomerServiceLinkService struct {
	generator urlLinkGenerator
	mu        sync.Mutex
	url       string
	refreshAt time.Time
	expiresAt time.Time
	now       func() time.Time
}

func NewCustomerServiceLinkService(generator urlLinkGenerator) *CustomerServiceLinkService {
	return &CustomerServiceLinkService{generator: generator, now: time.Now}
}

func (s *CustomerServiceLinkService) URL(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if s.url != "" && now.Before(s.refreshAt) {
		return s.url, nil
	}

	url, err := s.generator.GenerateURLLink(ctx, wechat.URLLinkRequest{
		Path:           customerServicePagePath,
		Query:          "",
		ExpireType:     1,
		ExpireInterval: 30,
		EnvVersion:     "release",
	})
	if err != nil {
		// A refresh failure must not break a still-valid cached link.
		if s.url != "" && now.Before(s.expiresAt) {
			return s.url, nil
		}
		return "", err
	}

	s.url = url
	s.refreshAt = now.Add(urlLinkRefreshAfter)
	s.expiresAt = now.Add(urlLinkValidity)
	return s.url, nil
}
