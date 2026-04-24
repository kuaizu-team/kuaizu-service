package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/kuaizu-team/kuaizu-service/internal/requestctx"
	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
)

// ContentAuditService 文字内容审核服务
type ContentAuditService struct {
	wxClient *wechat.Client
}

// NewContentAuditService creates a new ContentAuditService.
func NewContentAuditService(wxClient *wechat.Client) *ContentAuditService {
	return &ContentAuditService{
		wxClient: wxClient,
	}
}

// CheckText 校验文本内容是否合规。
// 传入多段文本，任一违规则返回 error。
func (s *ContentAuditService) CheckText(ctx context.Context, texts ...string) error {
	openID := requestctx.OpenIDFromContext(ctx)

	for _, text := range texts {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if openID == "" {
			log.Printf("[ContentAuditService.CheckText] missing openid in context")
			return ErrInternal("内容审核失败")
		}

		if err := s.wxClient.MsgSecCheck(ctx, openID, text); err != nil {
			if errors.Is(err, wechat.ErrContentBlocked) {
				return ErrBadRequest(fmt.Sprintf("内容[%s]包含违规信息，请修改后重试", text))
			}
			log.Printf("[ContentAuditService.CheckText] msgSecCheck error: %v", err)
			return ErrInternal("内容审核失败")
		}
	}

	return nil
}
