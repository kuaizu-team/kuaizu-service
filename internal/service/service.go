package service

import (
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

// Services aggregates all service instances.
type Services struct {
	Auth             *AuthService
	EmailPromotion   *EmailPromotionService
	SmsNotice        *SmsNoticeService
	Payment          *PaymentService
	EmailUnsubscribe *EmailUnsubscribeService
	Order            *OrderService
	OliveBranch      *OliveBranchService
	Commons          *CommonsService
	ContentAudit     *ContentAuditService
	TalentProfile    *TalentProfileService
	Project          *ProjectService
	Message          *MessageService
	User             *UserService
	Feedback         *FeedbackService
	Interaction      *InteractionService
}

// New creates a new Services instance with all sub-services.
func New(repo *repository.Repository, deps *Dependencies) *Services {
	contentAudit := NewContentAuditService(deps.WechatClient)
	message := NewMessageService(repo, deps.WechatClient)
	return &Services{
		Auth:             NewAuthService(repo, deps.WechatClient),
		EmailPromotion:   NewEmailPromotionServiceWithMessageCenter(repo, deps.MessageCenter, deps.MessageCenterInitError),
		SmsNotice:        NewSmsNoticeService(repo, deps.MessageCenter, deps.MessageCenterInitError),
		Payment:          NewPaymentService(repo, deps.PayClient, deps.PayInitError),
		EmailUnsubscribe: NewEmailUnsubscribeService(repo),
		Order:            NewOrderService(repo, deps.PayClient, deps.PayInitError),
		OliveBranch:      NewOliveBranchService(repo, message),
		Commons:          NewCommonsService(deps.OSSClient, repo.User),
		ContentAudit:     contentAudit,
		TalentProfile:    NewTalentProfileService(repo, contentAudit, message),
		Project:          NewProjectService(repo, contentAudit, message),
		Message:          message,
		User:             NewUserService(repo, message),
		Feedback:         NewFeedbackService(repo, message),
		Interaction:      NewInteractionService(repo),
	}
}

// truncate20 ensures s does not exceed the WeChat thing-type 20-character limit.
func truncate20(s string) string {
	r := []rune(s)
	if len(r) > 20 {
		return string(r[:20])
	}
	return s
}

// normalizePageParams enforces sane defaults for page/size.
func normalizePageParams(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}
	return page, size
}
