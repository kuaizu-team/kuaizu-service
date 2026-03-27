package service

import (
	"context"
	"log"
	"strings"

	"github.com/trv3wood/kuaizu-server/internal/wechat"
)

// ContentAuditService 文字内容审核服务
type ContentAuditService struct {
	wxClient *wechat.Client
}

// NewContentAuditService creates a new ContentAuditService.
func NewContentAuditService() *ContentAuditService {
	return &ContentAuditService{
		wxClient: wechat.NewClient(),
	}
}

// CheckText 校验文本内容是否合规。
// 传入多段文本，任一违规则返回 error。
func (s *ContentAuditService) CheckText(ctx context.Context, texts ...string) error {
	for _, text := range texts {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}

		if err := s.wxClient.MsgSecCheck(ctx, text); err != nil {
			log.Printf("[ContentAuditService.CheckText] msgSecCheck error: %v", err)
			return err
		}
	}

	return nil
}
