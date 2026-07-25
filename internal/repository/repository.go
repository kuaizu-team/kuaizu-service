package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// Repository aggregates all sub-repositories
type Repository struct {
	db                   *sqlx.DB
	User                 UserRepo
	Project              ProjectRepo
	Event                EventRepo
	Product              ProductRepo
	Roadmap              RoadmapRepo
	InformationContent   InformationContentRepo
	Recommendation       *RecommendationRepository
	Application          ApplicationRepo
	OliveBranch          OliveBranchRepo
	School               SchoolRepo
	Major                MajorRepo
	TalentProfile        TalentProfileRepo
	Order                OrderRepo
	EmailPromotion       EmailPromotionRepo
	SmsNotice            SmsNoticeRepo
	AdminUser            AdminUserRepo
	Feedback             FeedbackRepo
	InvitationFeedback   InvitationFeedbackRepo
	InvitationRecord     InvitationRecordRepo
	PendingInvitation    PendingInvitationRepo
	MsgTemplate          MsgTemplateConfigRepo
	SubscribeConfig      SubscribeConfigRepo
	ProjectViewLog       ProjectViewLogRepo
	TalentViewLog        TalentViewLogRepo
	Interaction          *InteractionRepository
	StatusNotification   StatusNotificationRepo
	WelcomeEmailDelivery WelcomeEmailDeliveryRepo
}

// DB returns the underlying database connection for transaction support
func (r *Repository) DB() *sqlx.DB {
	return r.db
}

// New creates a new Repository with all sub-repositories
func New(db *sqlx.DB) *Repository {
	return &Repository{
		db:                   db,
		User:                 NewUserRepository(db),
		Project:              NewProjectRepository(db),
		Event:                NewEventRepository(db),
		Product:              NewProductRepository(db),
		Roadmap:              NewRoadmapRepository(db),
		InformationContent:   NewInformationContentRepository(db),
		Recommendation:       NewRecommendationRepository(db),
		Application:          NewApplicationRepository(db),
		OliveBranch:          NewOliveBranchRepository(db),
		School:               NewSchoolRepository(db),
		Major:                NewMajorRepository(db),
		TalentProfile:        NewTalentProfileRepository(db),
		Order:                NewOrderRepository(db),
		EmailPromotion:       NewEmailPromotionRepository(db),
		SmsNotice:            NewSmsNoticeRepository(db),
		AdminUser:            NewAdminUserRepository(db),
		Feedback:             NewFeedbackRepository(db),
		InvitationFeedback:   NewInvitationFeedbackRepository(db),
		InvitationRecord:     NewInvitationRecordRepository(db),
		PendingInvitation:    NewPendingInvitationRepository(db),
		MsgTemplate:          NewMsgTemplateConfigRepository(db),
		SubscribeConfig:      NewSubscribeConfigRepository(db),
		ProjectViewLog:       NewProjectViewLogRepository(db),
		TalentViewLog:        NewTalentViewLogRepository(db),
		Interaction:          NewInteractionRepository(db),
		StatusNotification:   NewStatusNotificationRepository(db),
		WelcomeEmailDelivery: NewWelcomeEmailDeliveryRepository(db),
	}
}

// UpdateUserCollaborationScoreTx recalculates the user's collaboration score
// from frozen history and scores for currently active project memberships.
func UpdateUserCollaborationScoreTx(ctx context.Context, tx *sqlx.Tx, userID int) error {
	_, err := tx.ExecContext(ctx, `UPDATE `+"`user`"+` SET collaboration_score=(
		SELECT COALESCE(AVG(scores.score),90)
		FROM (
			SELECT cs.score
			FROM collaboration_score cs
			WHERE cs.user_id=?
			UNION ALL
			SELECT pms.score
			FROM project_member_score pms
			INNER JOIN project_members pm ON pm.id=pms.project_member_id
			WHERE pms.member_id=? AND pms.score IS NOT NULL
		) scores
	) WHERE id=?`, userID, userID, userID)
	return err
}
