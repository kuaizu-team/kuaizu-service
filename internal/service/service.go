package service

import (
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

// Services aggregates all service instances.
type Services struct {
	Auth                *AuthService
	AdminSms            *AdminSmsService
	EmailPromotion      *EmailPromotionService
	SmsNotice           *SmsNoticeService
	PushRetry           *PushRetryService
	Payment             *PaymentService
	EmailUnsubscribe    *EmailUnsubscribeService
	Order               *OrderService
	OliveBranch         *OliveBranchService
	Commons             *CommonsService
	ContentAudit        *ContentAuditService
	TalentProfile       *TalentProfileService
	Project             *ProjectService
	Event               *EventService
	Message             *MessageService
	User                *UserService
	Feedback            *FeedbackService
	Invitation          *InvitationFeedbackService
	RegisterInvitation  *RegisterInvitationService
	Interaction         *InteractionService
	WelcomeEmail        *WelcomeEmailService
	CustomerServiceLink *CustomerServiceLinkService
	ProjectDetailLink   *ProjectDetailLinkService
}

// New creates a new Services instance with all sub-services.
func New(repo *repository.Repository, deps *Dependencies) *Services {
	contentAudit := NewContentAuditService(deps.WechatClient)
	message := NewMessageService(repo, deps.WechatClient)
	emailPromotion := NewEmailPromotionServiceWithMessageCenter(repo, deps.MessageCenter, deps.MessageCenterInitError)
	smsNotice := NewSmsNoticeService(repo, deps.MessageCenter, deps.MessageCenterInitError)
	paidOrderDelivery := NewPaidOrderDeliveryService(repo, emailPromotion, smsNotice)
	payment := NewPaymentService(repo, deps.PayClient, deps.PayInitError)
	payment.SetPaidOrderDeliveryService(paidOrderDelivery)
	order := NewOrderService(repo, deps.PayClient, deps.PayInitError)
	order.ConfigureVirtualPayment(deps.WechatClient, deps.VirtualPayConfig, deps.VirtualPayInitError)
	return &Services{
		Auth:                NewAuthService(repo, deps.WechatClient),
		AdminSms:            NewAdminSmsService(deps.MessageCenter, deps.MessageCenterInitError),
		EmailPromotion:      emailPromotion,
		SmsNotice:           smsNotice,
		PushRetry:           NewPushRetryService(repo, emailPromotion, smsNotice),
		Payment:             payment,
		EmailUnsubscribe:    NewEmailUnsubscribeService(repo),
		Order:               order,
		OliveBranch:         NewOliveBranchService(repo, message),
		Commons:             NewCommonsService(deps.OSSClient, repo.User),
		ContentAudit:        contentAudit,
		TalentProfile:       NewTalentProfileService(repo, contentAudit, message),
		Project:             NewProjectService(repo, contentAudit, message),
		Event:               NewEventService(repo),
		Message:             message,
		User:                NewUserService(repo, message),
		Feedback:            NewFeedbackService(repo, message),
		Invitation:          NewInvitationFeedbackService(repo),
		RegisterInvitation:  NewRegisterInvitationService(repo, deps.MessageCenter, deps.MessageCenterInitError),
		Interaction:         NewInteractionService(repo, message),
		WelcomeEmail:        NewWelcomeEmailService(repo.WelcomeEmailDelivery, deps.MessageCenter, deps.MessageCenterInitError),
		CustomerServiceLink: NewCustomerServiceLinkService(deps.WechatClient),
		ProjectDetailLink:   NewProjectDetailLinkService(deps.WechatClient, repo.Project),
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

func truncate20WithEllipsis(s string) string {
	r := []rune(s)
	if len(r) > 20 {
		return string(r[:17]) + "..."
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
