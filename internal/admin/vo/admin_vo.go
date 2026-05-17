package vo

import (
	"math"
	"strconv"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/oss"
)

func intPtr(v int) *int { return &v }

// AdminProjectVO is the admin-facing project response model.
type AdminProjectVO struct {
	ID                   int          `json:"id"`
	CreatorID            int          `json:"creatorId"`
	Name                 string       `json:"name"`
	Description          *string      `json:"description"`
	SchoolID             *int         `json:"schoolId"`
	Direction            *int         `json:"direction"`
	MemberCount          *int         `json:"memberCount"`
	Status               int          `json:"status"`
	PromotionStatus      int          `json:"promotionStatus"`
	PromotionExpireTime  *time.Time   `json:"promotionExpireTime"`
	ViewCount            int          `json:"viewCount"`
	CreatedAt            time.Time    `json:"createdAt"`
	UpdatedAt            time.Time    `json:"updatedAt"`
	SchoolName           *string      `json:"schoolName"`
	IsCrossSchool        *int         `json:"isCrossSchool"`
	EducationRequirement *int         `json:"educationRequirement"`
	SkillRequirement     *string      `json:"skillRequirement"`
	Creator              *AdminUserVO `json:"creator,omitempty"`
	PendingCount         int          `json:"pendingCount"` // 投递+橄榄枝待处理总数
}

// AdminUserVO is the admin-facing user response model.
type AdminUserVO struct {
	ID                  int        `json:"id"`
	OpenID              string     `json:"openId"`
	Nickname            *string    `json:"nickname"`
	Phone               *string    `json:"phone"`
	Email               *string    `json:"email"`
	SchoolID            *int       `json:"schoolId"`
	MajorID             *int       `json:"majorId"`
	Grade               *int       `json:"grade"`
	LastActiveDate      *time.Time `json:"lastActiveDate"`
	AuthStatus          *int       `json:"authStatus"`
	AuthImgUrl          *string    `json:"authImgUrl"`
	AvatarUrl           *string    `json:"avatarUrl"`
	Wechat              *string    `json:"wechat"`
	EmailOptOut         *bool      `json:"emailOptOut"`
	CreatedAt           *time.Time `json:"createdAt"`
	SchoolName          *string    `json:"schoolName"`
	SchoolCode          *string    `json:"schoolCode"`
	MajorName           *string    `json:"majorName"`
	ClassID             *int       `json:"classId"`
	TalentProfileStatus *int       `json:"talentProfileStatus"`
	PendingCount        *int       `json:"pendingCount"` // 待审核投递数+待处理橄榄枝数
	UserStatus          int        `json:"userStatus"`   // 0=正常, 1=封禁, 2=已毕业
	BanReason           *string    `json:"banReason"`    // 封禁原因
}

// AdminTalentProfileVO is the admin-facing talent profile (business card) response model.
type AdminTalentProfileVO struct {
	ID                int      `json:"id"`
	Status            *int     `json:"status"`
	SelfEvaluation    *string  `json:"selfEvaluation"`
	ProjectExperience *string  `json:"projectExperience"`
	Skills            []string `json:"skills"`
	MBTI              *string  `json:"mbti"`
}

// AdminUserDetailVO extends AdminUserVO with the user's talent profile.
type AdminUserDetailVO struct {
	AdminUserVO
	TalentProfile *AdminTalentProfileVO `json:"talentProfile"`
}

// AdminFeedbackVO is the admin-facing feedback response model.
type AdminFeedbackVO struct {
	ID           int       `json:"id"`
	UserID       int       `json:"userId"`
	Content      string    `json:"content"`
	Email        *string   `json:"email"`
	Status       int       `json:"status"`
	AdminReply   *string   `json:"adminReply"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	UserNickname *string   `json:"userNickname"`
}

// NewAdminProjectVO converts a Project model to AdminProjectVO.
func NewAdminProjectVO(p *models.Project) *AdminProjectVO {
	if p == nil {
		return nil
	}

	adminProjectVo := AdminProjectVO{
		ID:                   p.ID,
		CreatorID:            p.CreatorID,
		Name:                 p.Name,
		Description:          p.Description,
		SchoolID:             p.SchoolID,
		Direction:            p.Direction,
		MemberCount:          p.MemberCount,
		Status:               p.Status,
		PromotionStatus:      p.PromotionStatus,
		PromotionExpireTime:  p.PromotionExpireTime,
		ViewCount:            p.ViewCount,
		CreatedAt:            p.CreatedAt,
		UpdatedAt:            p.UpdatedAt,
		SchoolName:           p.SchoolName,
		IsCrossSchool:        p.IsCrossSchool,
		EducationRequirement: p.EducationRequirement,
		SkillRequirement:     p.SkillRequirement,
		PendingCount:         p.PendingCount, // 0 when not computed (IncludePendingCount=false)
	}
	if p.Creator != nil {
		adminProjectVo.Creator = NewAdminUserVO(p.Creator, p.CreatorTalentProfileStatus)
	}
	return &adminProjectVo
}

// NewAdminUserVO converts a User model to AdminUserVO.
// talentProfileStatus is optional (pass nil when not available, e.g. in detail view where TalentProfile is fetched separately).
func NewAdminUserVO(u *models.User, talentProfileStatus *int) *AdminUserVO {
	if u == nil {
		return nil
	}

	pendingCount := u.PendingCount // 0 when not computed (IncludePendingCount=false)
	vo := AdminUserVO{
		ID:                  u.ID,
		OpenID:              u.OpenID,
		Nickname:            u.Nickname,
		Phone:               u.Phone,
		Email:               u.Email,
		SchoolID:            u.SchoolID,
		MajorID:             u.MajorID,
		Grade:               u.Grade,
		LastActiveDate:      u.LastActiveDate,
		AuthImgUrl:          ossFullURLPtr(u.AuthImgUrl),
		AvatarUrl:           ossFullURLPtr(u.AvatarUrl),
		Wechat:              u.WechatID,
		EmailOptOut:         u.EmailOptOut,
		CreatedAt:           u.CreatedAt,
		SchoolName:          u.SchoolName,
		SchoolCode:          u.SchoolCode,
		MajorName:           u.MajorName,
		ClassID:             u.ClassID,
		TalentProfileStatus: talentProfileStatus,
		PendingCount:        &pendingCount,
		UserStatus:          u.UserStatus,
		BanReason:           u.BanReason,
	}
	if u.AuthImgUrl != nil && u.AuthStatus != nil && *u.AuthStatus == 0 {
		vo.AuthStatus = intPtr(3) //  提交了审核材料且未认证，将状态映射为 3-审核中，方便管理员优先处理
	} else {
		vo.AuthStatus = u.AuthStatus
	}
	return &vo
}

// NewAdminFeedbackVO converts a Feedback model to AdminFeedbackVO.
func NewAdminFeedbackVO(f *models.Feedback) *AdminFeedbackVO {
	if f == nil {
		return nil
	}

	return &AdminFeedbackVO{
		ID:           f.ID,
		UserID:       f.UserID,
		Content:      f.Content,
		Email:        f.Email,
		Status:       f.Status,
		AdminReply:   f.AdminReply,
		CreatedAt:    f.CreatedAt,
		UpdatedAt:    f.UpdatedAt,
		UserNickname: f.UserNickname,
	}
}

// NewAdminTalentProfileVO converts a TalentProfile model to AdminTalentProfileVO.
func NewAdminTalentProfileVO(p *models.TalentProfile) *AdminTalentProfileVO {
	if p == nil {
		return nil
	}
	skills := []string{}
	if p.SkillSummary.Valid {
		skills = p.SkillSummary.Items
	}
	return &AdminTalentProfileVO{
		ID:                p.ID,
		Status:            p.Status,
		SelfEvaluation:    p.SelfEvaluation,
		ProjectExperience: p.ProjectExperience,
		Skills:            skills,
		MBTI:              p.MBTI,
	}
}

// NewAdminUserDetailVO converts a User and optional TalentProfile to AdminUserDetailVO.
func NewAdminUserDetailVO(u *models.User, p *models.TalentProfile) *AdminUserDetailVO {
	if u == nil {
		return nil
	}
	return &AdminUserDetailVO{
		AdminUserVO:   *NewAdminUserVO(u, nil),
		TalentProfile: NewAdminTalentProfileVO(p),
	}
}

// AdminApplicationVO is the admin-facing project application response model.
type AdminApplicationVO struct {
	ID          int       `json:"id"`
	ProjectID   int       `json:"projectId"`
	ProjectName *string   `json:"projectName"`
	Status      int       `json:"status"`
	AppliedAt   time.Time `json:"appliedAt"`
}

// NewAdminApplicationVO converts a ProjectApplication model to AdminApplicationVO.
func NewAdminApplicationVO(a *models.ProjectApplication) *AdminApplicationVO {
	if a == nil {
		return nil
	}
	return &AdminApplicationVO{
		ID:          a.ID,
		ProjectID:   a.ProjectID,
		ProjectName: a.ProjectName,
		Status:      a.Status,
		AppliedAt:   a.AppliedAt,
	}
}

// AdminOliveBranchVO is the admin-facing olive branch response model.
type AdminOliveBranchVO struct {
	ID               int       `json:"id"`
	RelatedProjectID *int      `json:"relatedProjectId"`
	ProjectName      *string   `json:"projectName"`
	Type             int       `json:"type"`
	SenderNickname   *string   `json:"senderNickname"`
	Status           int       `json:"status"`
	CreatedAt        time.Time `json:"createdAt"`
}

// NewAdminOliveBranchVO converts an OliveBranch model to AdminOliveBranchVO.
func NewAdminOliveBranchVO(ob *models.OliveBranch) *AdminOliveBranchVO {
	if ob == nil {
		return nil
	}
	var relatedProjectID *int
	if ob.RelatedProjectID != 0 {
		id := ob.RelatedProjectID
		relatedProjectID = &id
	}
	var senderNickname *string
	if ob.Sender != nil {
		senderNickname = ob.Sender.Nickname
	}
	return &AdminOliveBranchVO{
		ID:               ob.ID,
		RelatedProjectID: relatedProjectID,
		ProjectName:      ob.ProjectName,
		Type:             ob.Type,
		SenderNickname:   senderNickname,
		Status:           ob.Status,
		CreatedAt:        ob.CreatedAt,
	}
}

// AdminProjectApplicantVO is the admin-facing project application with full applicant info,
// used by GET /admin/projects/:id/applications.
type AdminProjectApplicantVO struct {
	ID        int          `json:"id"`
	UserID    int          `json:"userId"`
	Status    int          `json:"status"`
	AppliedAt time.Time    `json:"appliedAt"`
	Applicant *AdminUserVO `json:"applicant"`
}

// NewAdminProjectApplicantVO converts a ProjectApplication model to AdminProjectApplicantVO.
// talentProfileStatus is the applicant's talent profile status (nil if unknown).
func NewAdminProjectApplicantVO(a *models.ProjectApplication, talentProfileStatus *int) *AdminProjectApplicantVO {
	if a == nil {
		return nil
	}
	vo := &AdminProjectApplicantVO{
		ID:        a.ID,
		UserID:    a.UserID,
		Status:    a.Status,
		AppliedAt: a.AppliedAt,
	}
	if a.Applicant != nil {
		vo.Applicant = NewAdminUserVO(a.Applicant, talentProfileStatus)
	}
	return vo
}

// AdminProjectOliveBranchVO is the admin-facing olive branch record with full receiver info,
// used by GET /admin/projects/:id/olive-branches.
type AdminProjectOliveBranchVO struct {
	ID        int          `json:"id"`
	Type      int          `json:"type"`
	Status    int          `json:"status"`
	CreatedAt time.Time    `json:"createdAt"`
	Receiver  *AdminUserVO `json:"receiver"`
}

// NewAdminProjectOliveBranchVO converts an OliveBranch model to AdminProjectOliveBranchVO.
// talentProfileStatus is the receiver's talent profile status (nil if unknown).
func NewAdminProjectOliveBranchVO(ob *models.OliveBranch, talentProfileStatus *int) *AdminProjectOliveBranchVO {
	if ob == nil {
		return nil
	}
	vo := &AdminProjectOliveBranchVO{
		ID:        ob.ID,
		Type:      ob.Type,
		Status:    ob.Status,
		CreatedAt: ob.CreatedAt,
	}
	if ob.Receiver != nil {
		vo.Receiver = NewAdminUserVO(ob.Receiver, talentProfileStatus)
	}
	return vo
}

// AdminUserAccountVO is the admin-facing admin-user response model (管理员账号).
type AdminUserAccountVO struct {
	ID         int        `json:"id"`
	Username   string     `json:"username"`
	Nickname   *string    `json:"nickname"`
	Role       int        `json:"role"`
	SchoolID   *int       `json:"schoolId"`
	SchoolName *string    `json:"schoolName"`
	Status     int        `json:"status"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// NewAdminUserAccountVO converts an AdminUser model to AdminUserAccountVO (no password).
func NewAdminUserAccountVO(a *models.AdminUser) *AdminUserAccountVO {
	if a == nil {
		return nil
	}
	return &AdminUserAccountVO{
		ID:         a.ID,
		Username:   a.Username,
		Nickname:   a.Nickname,
		Role:       a.Role,
		SchoolID:   a.SchoolID,
		SchoolName: a.SchoolName,
		Status:     a.Status,
		CreatedAt:  a.CreatedAt,
		UpdatedAt:  a.UpdatedAt,
	}
}

// AdminOrderVO is the admin-facing order response model (list item).
type AdminOrderVO struct {
	ID           int        `json:"id"`
	OutTradeNo   string     `json:"outTradeNo"` // 使用订单 ID 的字符串形式（out_trade_no 列未使用）
	WxPayNo      *string    `json:"wxPayNo"`
	UserID       int        `json:"userId"`
	UserNickname *string    `json:"userNickname"`
	SchoolName   *string    `json:"schoolName"`
	ProductID    int        `json:"productId"`
	ProductName  *string    `json:"productName"`
	Quantity     int        `json:"quantity"`
	UnitPrice    int        `json:"unitPrice"`  // 单价，单位：分
	ActualPaid   int        `json:"actualPaid"` // 实付，单位：分
	Status       int        `json:"status"`     // 0=待支付 1=支付成功 2=支付失败 3=已退款
	CreatedAt    time.Time  `json:"createdAt"`
	PaidAt       *time.Time `json:"paidAt"`
}

// AdminOrderDetailVO extends AdminOrderVO with detail-only fields.
type AdminOrderDetailVO struct {
	AdminOrderVO
	Description *string   `json:"description"` // 商品描述，允许为 null
	UpdatedAt   time.Time `json:"updatedAt"`
}

// NewAdminOrderVO converts an Order model to AdminOrderVO.
func NewAdminOrderVO(o *models.Order) *AdminOrderVO {
	if o == nil {
		return nil
	}
	return &AdminOrderVO{
		ID:           o.ID,
		OutTradeNo:   strconv.Itoa(o.ID), // out_trade_no 列未使用，以 ID 字符串作为订单号
		WxPayNo:      o.WxPayNo,
		UserID:       o.UserID,
		UserNickname: o.UserNickname,
		SchoolName:   o.SchoolName,
		ProductID:    o.ProductID,
		ProductName:  o.ProductName,
		Quantity:     o.Quantity,
		UnitPrice:    int(math.Round(o.Price * 100)),
		ActualPaid:   int(math.Round(o.ActualPaid * 100)),
		Status:       o.Status,
		CreatedAt:    o.CreatedAt,
		PaidAt:       o.PayTime,
	}
}

// NewAdminOrderDetailVO converts an Order model to AdminOrderDetailVO.
func NewAdminOrderDetailVO(o *models.Order) *AdminOrderDetailVO {
	if o == nil {
		return nil
	}
	return &AdminOrderDetailVO{
		AdminOrderVO: *NewAdminOrderVO(o),
		Description:  o.ProductDescription,
		UpdatedAt:    o.UpdatedAt,
	}
}

// ossFullURLPtr resolves a nullable relative OSS path to a full URL pointer.
func ossFullURLPtr(rel *string) *string {
	if rel == nil {
		return nil
	}
	v := oss.FullURL(*rel)
	return &v
}
