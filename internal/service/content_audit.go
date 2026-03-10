package service

import "context"

// ContentAuditService 文字内容审核服务
type ContentAuditService struct{}

// NewContentAuditService creates a new ContentAuditService.
func NewContentAuditService() *ContentAuditService {
	return &ContentAuditService{}
}

// CheckText 校验文本内容是否合规。
// 传入多段文本，任一违规则返回 error。
// TODO: 接入微信 msgSecCheck 或第三方内容安全 API
func (s *ContentAuditService) CheckText(ctx context.Context, texts ...string) error {
	// 过滤空字符串
	// for _, t := range texts {
	// 	if t == "" {
	// 		continue
	// 	}
	// 	// 调用内容安全 API ...
	// }
	return nil
}
