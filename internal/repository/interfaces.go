package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/trv3wood/kuaizu-server/api"
	"github.com/trv3wood/kuaizu-server/internal/models"
)

// OrderRepo defines the interface for order repository operations used by services.
type OrderRepo interface {
	GetByID(ctx context.Context, id int) (*models.Order, error)
	Create(ctx context.Context, order *models.Order) (*models.Order, error)
	ListByUserID(ctx context.Context, params OrderListParams) ([]*models.Order, int64, error)
	UpdatePaymentStatus(ctx context.Context, id int, status int, wxPayNo string, payTime time.Time) error
	UpdatePaymentStatusTx(ctx context.Context, tx *sqlx.Tx, id int, status int, wxPayNo string, payTime time.Time) error
	GetOrderItems(ctx context.Context, orderID int) ([]*models.OrderItem, error)
	UpdateStatus(ctx context.Context, id int, status int) error
}

// ProjectRepo defines the interface for project repository operations used by services.
type ProjectRepo interface {
	GetByID(ctx context.Context, id int) (*models.Project, error)
	List(ctx context.Context, params ListParams) ([]models.Project, int64, error)
	Create(ctx context.Context, p *models.Project) error
	Update(ctx context.Context, p *models.Project) error
	Delete(ctx context.Context, id int) error
	IsOwner(ctx context.Context, projectID, userID int) (bool, error)
	UpdateStatus(ctx context.Context, id int, status int) error
	IncrementViewCount(ctx context.Context, id int) error
}

// ProductRepo defines the interface for product repository operations used by services.
type ProductRepo interface {
	GetByID(ctx context.Context, id int) (*models.Product, error)
	GetAll(ctx context.Context) ([]*models.Product, error)
}

// EmailPromotionRepo defines the interface for email promotion repository operations.
type EmailPromotionRepo interface {
	Create(ctx context.Context, promotion *models.EmailPromotion) error
	GetByID(ctx context.Context, id int) (*models.EmailPromotion, error)
	GetByOrderID(ctx context.Context, orderID int) (*models.EmailPromotion, error)
	Update(ctx context.Context, promotion *models.EmailPromotion) error
	ListByCreatorID(ctx context.Context, creatorID int, page, size int) ([]models.EmailPromotion, int64, error)
	ListByProjectID(ctx context.Context, projectID int) ([]models.EmailPromotion, error)
}

// UserRepo defines the interface for user repository operations used by services.
type UserRepo interface {
	GetByID(ctx context.Context, id int) (*models.User, error)
	GetByOpenID(ctx context.Context, openid string) (*models.User, error)
	Create(ctx context.Context, openid string) (*models.User, error)
	CreateWithPhone(ctx context.Context, openid string, phone string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	UpdatePhone(ctx context.Context, userID int, phone string) error
	UpdateQuota(ctx context.Context, user *models.User) error
	AddOliveBranchCount(ctx context.Context, userID int, count int) error
	AddOliveBranchCountTx(ctx context.Context, tx *sqlx.Tx, userID int, count int) error
	UpdateAuthStatus(ctx context.Context, userID int, authStatus int) error
	ListUsers(ctx context.Context, params UserListParams) ([]models.User, int64, error)
	FindEmailRecipients(ctx context.Context, excludeUserID int, limit int) ([]*EmailRecipient, error)
	SetEmailOptOut(ctx context.Context, userID int, optOut bool) error
	UpdateAuthImgUrl(ctx context.Context, userID int, authImgUrl string) error
	UpdateAvatarUrl(ctx context.Context, userID int, avatarUrl string) error
	UpdateCoverImage(ctx context.Context, userID int, coverImage string) error
	GetEduCertInfoByID(ctx context.Context, userID int) (CertInfo, error)
}

// ApplicationRepo defines the interface for application repository operations.
type ApplicationRepo interface {
	List(ctx context.Context, params ApplicationListParams) ([]models.ProjectApplication, int64, error)
	Create(ctx context.Context, app *models.ProjectApplication) error
	GetByID(ctx context.Context, id int) (*models.ProjectApplication, error)
	CheckDuplicate(ctx context.Context, projectID, userID int) (bool, error)
	UpdateStatus(ctx context.Context, id int, status int) error
}

// OliveBranchRepo defines the interface for olive branch repository operations.
type OliveBranchRepo interface {
	ListByReceiverID(ctx context.Context, params OliveBranchListParams) ([]models.OliveBranch, int64, error)
	GetByID(ctx context.Context, id int) (*models.OliveBranch, error)
	Create(ctx context.Context, ob *models.OliveBranch) error
	UpdateStatus(ctx context.Context, id int, status int) error
	ListBySenderID(ctx context.Context, params OliveBranchListParams) ([]models.OliveBranch, int64, error)
	ExistsPending(ctx context.Context, senderID, receiverID, relatedProjectID int) (bool, error)
}

// SchoolRepo defines the interface for school repository operations.
type SchoolRepo interface {
	List(ctx context.Context, keyword *string) ([]*models.School, error)
}

// MajorRepo defines the interface for major repository operations.
type MajorRepo interface {
	List(ctx context.Context, params *api.ListMajorsParams) ([]models.Major, error)
	ListWithMajors(ctx context.Context, params api.ListMajorsParams) ([]models.MajorClass, error)
}

// TalentProfileRepo defines the interface for talent profile repository operations.
type TalentProfileRepo interface {
	List(ctx context.Context, params TalentProfileListParams) ([]models.TalentProfile, int64, error)
	GetByID(ctx context.Context, id int) (*models.TalentProfile, error)
	GetByUserID(ctx context.Context, userID int) (*models.TalentProfile, error)
	Upsert(ctx context.Context, p *models.TalentProfile) error
	DeleteByUserID(ctx context.Context, userID int) error
}

// AdminUserRepo defines the interface for admin user repository operations.
type AdminUserRepo interface {
	GetByUsername(ctx context.Context, username string) (*models.AdminUser, error)
	GetByID(ctx context.Context, id int) (*models.AdminUser, error)
}

// FeedbackRepo defines the interface for feedback repository operations.
type FeedbackRepo interface {
	List(ctx context.Context, params FeedbackListParams) ([]models.Feedback, int64, error)
	GetByID(ctx context.Context, id int) (*models.Feedback, error)
	Reply(ctx context.Context, id int, reply string) error
}

// Compile-time interface satisfaction checks
var _ OrderRepo = (*OrderRepository)(nil)
var _ ProjectRepo = (*ProjectRepository)(nil)
var _ ProductRepo = (*ProductRepository)(nil)
var _ EmailPromotionRepo = (*EmailPromotionRepository)(nil)
var _ UserRepo = (*UserRepository)(nil)
var _ ApplicationRepo = (*ApplicationRepository)(nil)
var _ OliveBranchRepo = (*OliveBranchRepository)(nil)
var _ SchoolRepo = (*SchoolRepository)(nil)
var _ MajorRepo = (*MajorRepository)(nil)
var _ TalentProfileRepo = (*TalentProfileRepository)(nil)
var _ AdminUserRepo = (*AdminUserRepository)(nil)
var _ FeedbackRepo = (*FeedbackRepository)(nil)
