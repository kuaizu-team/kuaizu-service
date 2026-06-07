package repository

import (
	"github.com/jmoiron/sqlx"
)

// Repository aggregates all sub-repositories
type Repository struct {
	db                 *sqlx.DB
	User               UserRepo
	Project            ProjectRepo
	Product            ProductRepo
	InformationContent InformationContentRepo
	Application        ApplicationRepo
	OliveBranch        OliveBranchRepo
	School             SchoolRepo
	Major              MajorRepo
	TalentProfile      TalentProfileRepo
	Order              OrderRepo
	EmailPromotion     EmailPromotionRepo
	SmsNotice          SmsNoticeRepo
	AdminUser          AdminUserRepo
	Feedback           FeedbackRepo
	InvitationFeedback InvitationFeedbackRepo
	MsgTemplate        MsgTemplateConfigRepo
	SubscribeConfig    SubscribeConfigRepo
	ProjectViewLog     ProjectViewLogRepo
	TalentViewLog      TalentViewLogRepo
	Interaction        *InteractionRepository
}

// DB returns the underlying database connection for transaction support
func (r *Repository) DB() *sqlx.DB {
	return r.db
}

// New creates a new Repository with all sub-repositories
func New(db *sqlx.DB) *Repository {
	return &Repository{
		db:                 db,
		User:               NewUserRepository(db),
		Project:            NewProjectRepository(db),
		Product:            NewProductRepository(db),
		InformationContent: NewInformationContentRepository(db),
		Application:        NewApplicationRepository(db),
		OliveBranch:        NewOliveBranchRepository(db),
		School:             NewSchoolRepository(db),
		Major:              NewMajorRepository(db),
		TalentProfile:      NewTalentProfileRepository(db),
		Order:              NewOrderRepository(db),
		EmailPromotion:     NewEmailPromotionRepository(db),
		SmsNotice:          NewSmsNoticeRepository(db),
		AdminUser:          NewAdminUserRepository(db),
		Feedback:           NewFeedbackRepository(db),
		InvitationFeedback: NewInvitationFeedbackRepository(db),
		MsgTemplate:        NewMsgTemplateConfigRepository(db),
		SubscribeConfig:    NewSubscribeConfigRepository(db),
		ProjectViewLog:     NewProjectViewLogRepository(db),
		TalentViewLog:      NewTalentViewLogRepository(db),
		Interaction:        NewInteractionRepository(db),
	}
}
