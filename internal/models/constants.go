package models

// Admin User Status
const (
	AdminUserStatusDisabled = 0 // 禁用
	AdminUserStatusEnabled  = 1 // 启用
)

// Email Promotion Status
const (
	EmailPromotionStatusPending   EmailPromotionStatus = 0 // 待发送
	EmailPromotionStatusSending   EmailPromotionStatus = 1 // 发送中
	EmailPromotionStatusCompleted EmailPromotionStatus = 2 // 已完成
	EmailPromotionStatusFailed    EmailPromotionStatus = 3 // 失败
)

// Feedback Status
const (
	FeedbackStatusPending = 0 // 待处理
	FeedbackStatusDone    = 1 // 已处理
)

// Olive Branch Status
const (
	OliveBranchStatusPending  = 0 // 待处理
	OliveBranchStatusAccepted = 1 // 已接受
	OliveBranchStatusRejected = 2 // 已拒绝
	OliveBranchStatusIgnored  = 3 // 已忽略
)

// Olive Branch Cost Type
const (
	OliveBranchCostFree = 1 // 免费额度
	OliveBranchCostPaid = 2 // 付费额度
)

// Olive Branch Type
const (
	OliveBranchTypeTalent = 1 // 人才互联
)

// Order Status
const (
	OrderStatusPending   = 0 // 待支付
	OrderStatusPaid      = 1 // 已支付
	OrderStatusCancelled = 2 // 已取消
	OrderStatusRefunded  = 3 // 已退款
)

// Product Type
const (
	ProductTypeCurrency = 1 // 虚拟币
	ProductTypeBenefit  = 2 // 服务权益
)

// Project Direction
const (
	ProjectDirectionLaunch      = 1 // 落地
	ProjectDirectionCompetition = 2 // 比赛
	ProjectDirectionLearning    = 3 // 学习
)

// Project Status
const (
	ProjectStatusPending  = 0 // 待审核
	ProjectStatusApproved = 1 // 已通过/进行中
	ProjectStatusRejected = 2 // 已驳回
	ProjectStatusClosed   = 3 // 已关闭
)

// Project Promotion Status
const (
	ProjectPromotionNone     = 0 // 无
	ProjectPromotionActive   = 1 // 推广中
	ProjectPromotionFinished = 2 // 已结束
)

// Project Cross School
const (
	ProjectCrossSchoolNo  = 0 // 可以单独申请本校
	ProjectCrossSchoolYes = 1 // 可以申请外校
)

// Project Education Requirement
const (
	EducationJuniorCollege = 1 // 大专
	EducationUndergraduate = 2 // 本科
	EducationPostgraduate  = 3 // 研究生
)

// Project Application Status
const (
	ApplicationStatusPending  = 0 // 待审核
	ApplicationStatusApproved = 1 // 已通过
	ApplicationStatusRejected = 2 // 已拒绝
)

// Talent Profile Status
const (
	TalentStatusPrivate   = 0 // 隐私/下架
	TalentStatusOnline    = 1 // 上架
	TalentStatusReviewing = 2 // 审核中
)

// User Auth Status
const (
	UserAuthStatusNone   = 0 // 未认证
	UserAuthStatusPassed = 1 // 已认证
	UserAuthStatusFailed = 2 // 认证失败
)

// Message Business Keys (Subscription Messages)
const (
	MsgBizKeyCardReceived       = "MSG_CARD_RECEIVED"        // 收到名片通知
	MsgBizKeyCardDeliveryResult = "MSG_CARD_DELIVERY_RESULT" // 名片投递结果通知
	MsgBizKeyAuditResultProj    = "MSG_AUDIT_RESULT_PROJ"    // 审核结果通知(项目)
	MsgBizKeyUserReply          = "MSG_USER_REPLY"           // 用户回复结果通知
	MsgBizKeyInviteJoin         = "MSG_INVITE_JOIN"          // 邀请加入项目通知
	MsgBizKeyAuditResultUser    = "MSG_AUDIT_RESULT_USER"    // 审核结果通知(个人)
	MsgBizKeyIdentityAuth       = "MSG_IDENTITY_AUTH"        // 身份认证通知
)
