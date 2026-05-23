package repository

import (
	"github.com/jmoiron/sqlx"
)

// Repository aggregates all sub-repositories
type Repository struct {
	db              *sqlx.DB
	User            UserRepo
	Project         ProjectRepo
	Product         ProductRepo
	Application     ApplicationRepo
	OliveBranch     OliveBranchRepo
	School          SchoolRepo
	Major           MajorRepo
	TalentProfile   TalentProfileRepo
	Order           OrderRepo
	EmailPromotion  EmailPromotionRepo
	AdminUser       AdminUserRepo
	Feedback        FeedbackRepo
	MsgTemplate     MsgTemplateConfigRepo
	SubscribeConfig SubscribeConfigRepo
	ProjectViewLog  ProjectViewLogRepo
	TalentViewLog   TalentViewLogRepo
}

// DB returns the underlying database connection for transaction support
func (r *Repository) DB() *sqlx.DB {
	return r.db
}

// New creates a new Repository with all sub-repositories
func New(db *sqlx.DB) *Repository {
	return &Repository{
		db:              db,
		User:            NewUserRepository(db),
		Project:         NewProjectRepository(db),
		Product:         NewProductRepository(db),
		Application:     NewApplicationRepository(db),
		OliveBranch:     NewOliveBranchRepository(db),
		School:          NewSchoolRepository(db),
		Major:           NewMajorRepository(db),
		TalentProfile:   NewTalentProfileRepository(db),
		Order:           NewOrderRepository(db),
		EmailPromotion:  NewEmailPromotionRepository(db),
		AdminUser:       NewAdminUserRepository(db),
		Feedback:        NewFeedbackRepository(db),
		MsgTemplate:     NewMsgTemplateConfigRepository(db),
		SubscribeConfig: NewSubscribeConfigRepository(db),
		ProjectViewLog:  NewProjectViewLogRepository(db),
		TalentViewLog:   NewTalentViewLogRepository(db),
	}
}
