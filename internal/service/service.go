package service

import (
	"github.com/trv3wood/kuaizu-server/internal/oss"
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
	Project          *ProjectService
}

// New creates a new Services instance with all sub-services.
func New(repo *repository.Repository, ossClient *oss.Client) *Services {
	contentAudit := NewContentAuditService()
	return &Services{
		Auth:             NewAuthService(repo),
		EmailPromotion:   NewEmailPromotionService(repo),
		Payment:          NewPaymentService(repo),
		EmailUnsubscribe: NewEmailUnsubscribeService(repo),
		Order:            NewOrderService(repo),
		OliveBranch:      NewOliveBranchService(repo),
		Commons:          NewCommonsService(ossClient, repo.User),
		ContentAudit:     contentAudit,
		Project:          NewProjectService(repo, contentAudit),
	}
}
