package service

import (
	"github.com/trv3wood/kuaizu-server/internal/repository"
)

// Services aggregates all service instances.
type Services struct {
	Auth             *AuthService
	EmailPromotion   *EmailPromotionService
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
}

// New creates a new Services instance with all sub-services.
func New(repo *repository.Repository, deps *Dependencies) *Services {
	contentAudit := NewContentAuditService(deps.WechatClient)
	message := NewMessageService(repo, deps.WechatClient)
	return &Services{
		Auth:             NewAuthService(repo, deps.WechatClient),
		EmailPromotion:   NewEmailPromotionServiceWithEmail(repo, deps.EmailService, deps.EmailInitError),
		Payment:          NewPaymentService(repo, deps.PayClient, deps.PayInitError),
		EmailUnsubscribe: NewEmailUnsubscribeService(repo),
		Order:            NewOrderService(repo, deps.PayClient, deps.PayInitError),
		OliveBranch:      NewOliveBranchService(repo),
		Commons:          NewCommonsService(deps.OSSClient, repo.User),
		ContentAudit:     contentAudit,
		TalentProfile:    NewTalentProfileService(repo, contentAudit),
		Project:          NewProjectService(repo, contentAudit, message),
		Message:          message,
		User:             NewUserService(repo, message),
		Feedback:         NewFeedbackService(repo, message),
	}
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
