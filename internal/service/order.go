package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
)

// OrderService handles order-related business logic.
type OrderService struct {
	repo              *repository.Repository
	payClient         *wechat.PayClient
	payInitErr        error
	wxClient          *wechat.Client
	virtualPayConfig  *wechat.VirtualPayConfig
	virtualPayInitErr error
}

// ConfigureVirtualPayment enables the isolated official Mini Program virtual-payment flow.
func (s *OrderService) ConfigureVirtualPayment(wxClient *wechat.Client, config *wechat.VirtualPayConfig, initErr error) {
	s.wxClient = wxClient
	s.virtualPayConfig = config
	s.virtualPayInitErr = initErr
}

// ApplyRefund submits a refund request for the current user's paid order.
func (s *OrderService) ApplyRefund(ctx context.Context, userID, orderID int, reason string) (*models.Order, error) {
	reason = strings.TrimSpace(reason)
	if len([]rune(reason)) < 5 || len([]rune(reason)) > 200 {
		return nil, ErrBadRequest("退款原因需为5-200字")
	}

	order, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("[OrderService.ApplyRefund] repository error getting order: %v", err)
		return nil, ErrInternal("获取订单详情失败")
	}
	if order == nil {
		return nil, ErrNotFound("订单不存在")
	}
	if order.UserID != userID {
		return nil, ErrForbidden("无权操作此订单")
	}
	if order.Status != models.OrderStatusPaid {
		return nil, ErrBadRequest("未支付订单不能申请退款")
	}
	if order.RefundStatus != 0 {
		return nil, ErrBadRequest("该订单已申请退款")
	}

	ok, err := s.repo.Order.UpdateRefundApply(ctx, orderID, reason, 0, nil)
	if err != nil {
		log.Printf("[OrderService.ApplyRefund] repository error updating refund apply: %v", err)
		return nil, ErrInternal("提交退款申请失败")
	}
	if !ok {
		return nil, ErrBadRequest("订单状态不允许申请退款")
	}

	updated, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("[OrderService.ApplyRefund] repository error getting updated order: %v", err)
		return nil, ErrInternal("获取更新后的订单失败")
	}

	return updated, nil
}

// WithdrawRefund withdraws the current user's pending refund request.
func (s *OrderService) WithdrawRefund(ctx context.Context, userID, orderID int) (*models.Order, error) {
	order, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("[OrderService.WithdrawRefund] repository error getting order: %v", err)
		return nil, ErrInternal("获取订单详情失败")
	}
	if order == nil {
		return nil, ErrNotFound("订单不存在")
	}
	if order.UserID != userID {
		return nil, ErrForbidden("无权操作此订单")
	}
	if order.RefundStatus != 1 {
		return nil, ErrBadRequest("只能撤回待退款申请")
	}

	ok, err := s.repo.Order.WithdrawRefund(ctx, orderID)
	if err != nil {
		log.Printf("[OrderService.WithdrawRefund] repository error withdrawing refund: %v", err)
		return nil, ErrInternal("撤回退款申请失败")
	}
	if !ok {
		return nil, ErrBadRequest("只能撤回待退款申请")
	}

	updated, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("[OrderService.WithdrawRefund] repository error getting updated order: %v", err)
		return nil, ErrInternal("获取更新后的订单失败")
	}

	return updated, nil
}

// NewOrderService creates a new OrderService.
func NewOrderService(repo *repository.Repository, payClient *wechat.PayClient, payInitErr error) *OrderService {
	return &OrderService{
		repo:       repo,
		payClient:  payClient,
		payInitErr: payInitErr,
	}
}

// CreateOrderItem is the input DTO for creating an order.
type CreateOrderItem struct {
	ProductID int
	Quantity  int
	Delivery  *models.OrderDeliveryIntent
}

// CreateOrder validates product, calculates price, and creates an order.
func (s *OrderService) CreateOrder(ctx context.Context, userID int, item CreateOrderItem) (*models.Order, error) {
	if item.ProductID <= 0 {
		return nil, ErrBadRequest("商品ID无效")
	}
	if item.Quantity <= 0 {
		return nil, ErrBadRequest("购买数量必须大于0")
	}

	product, err := s.repo.Product.GetByID(ctx, item.ProductID)
	if err != nil {
		log.Printf("[OrderService.CreateOrder] repository error getting product: %v", err)
		return nil, ErrInternal("获取商品信息失败")
	}
	if product == nil {
		return nil, ErrNotFound(fmt.Sprintf("商品ID %d 不存在", item.ProductID))
	}

	var deliveryScene, deliveryPayload *string
	if item.Delivery != nil {
		if err := s.validateDeliveryIntent(ctx, userID, product, item.Delivery); err != nil {
			return nil, err
		}
		payload, err := json.Marshal(item.Delivery)
		if err != nil {
			return nil, ErrInternal("创建订单交付信息失败")
		}
		deliveryScene = &item.Delivery.Scene
		encoded := string(payload)
		deliveryPayload = &encoded
	}

	actualPaid := product.Price * float64(item.Quantity)

	order := &models.Order{
		UserID:          userID,
		ProductID:       item.ProductID,
		Price:           product.Price,
		Quantity:        item.Quantity,
		ActualPaid:      actualPaid,
		Status:          models.OrderStatusPending,
		DeliveryScene:   deliveryScene,
		DeliveryPayload: deliveryPayload,
	}

	createdOrder, err := s.repo.Order.Create(ctx, order)
	if err != nil {
		log.Printf("[OrderService.CreateOrder] repository error creating order: %v", err)
		return nil, ErrInternal("创建订单失败")
	}

	return createdOrder, nil
}

func (s *OrderService) validateDeliveryIntent(ctx context.Context, userID int, product *models.Product, intent *models.OrderDeliveryIntent) error {
	if product.Type != models.ProductTypeBenefit {
		return ErrBadRequest("该商品不支持支付后自动交付")
	}
	switch intent.Scene {
	case models.OrderDeliverySceneEmailPromotion:
		if isSmsNoticeProduct(product) || intent.ProjectID == nil || *intent.ProjectID <= 0 {
			return ErrBadRequest("邮件推广交付参数无效")
		}
		intent.Strategy = normalizePromotionStrategy(intent.Strategy)
		if !isValidPromotionStrategy(intent.Strategy) {
			return ErrBadRequest("邮件推广策略无效")
		}
		project, err := s.repo.Project.GetByID(ctx, *intent.ProjectID)
		if err != nil {
			return ErrInternal("获取推广项目失败")
		}
		if project == nil || project.CreatorID != userID {
			return ErrForbidden("只能推广自己创建的项目")
		}
	case models.OrderDeliverySceneSMSNotice:
		if !isSmsNoticeProduct(product) || intent.ReceiverUserID == nil || *intent.ReceiverUserID <= 0 {
			return ErrBadRequest("短信通知交付参数无效")
		}
		referenceCount := 0
		if intent.OliveBranchRecordID != nil && *intent.OliveBranchRecordID > 0 {
			referenceCount++
		}
		if intent.ApplicationID != nil && *intent.ApplicationID > 0 {
			referenceCount++
		}
		if intent.MemberRemovalID != nil && *intent.MemberRemovalID > 0 {
			referenceCount++
		}
		if referenceCount != 1 {
			return ErrBadRequest("短信通知必须且只能关联一条业务记录")
		}
		if intent.NoticeType != nil {
			trimmed := strings.TrimSpace(*intent.NoticeType)
			intent.NoticeType = &trimmed
		}
		if err := s.validateSmsDeliveryIntent(ctx, userID, intent); err != nil {
			return err
		}
	default:
		return ErrBadRequest("不支持的订单交付场景")
	}
	return nil
}

// validateSmsDeliveryIntent rejects business-invalid delivery intents before an order can be paid.
// Mutable business state is checked again by SmsNoticeService when delivery actually runs.
func (s *OrderService) validateSmsDeliveryIntent(ctx context.Context, userID int, intent *models.OrderDeliveryIntent) error {
	if intent == nil || intent.ReceiverUserID == nil || *intent.ReceiverUserID <= 0 {
		return ErrBadRequest("短信通知交付参数无效")
	}
	receiverID := *intent.ReceiverUserID
	validateReceiver := func(expectedID int) error {
		if receiverID != expectedID {
			return ErrBadRequest("receiverUserId与业务记录不匹配")
		}
		receiver, err := s.repo.User.GetByID(ctx, expectedID)
		if err != nil {
			return ErrInternal("获取短信接收人失败")
		}
		if !hasValidMainlandPhone(receiver) {
			return ErrBadRequest("短信接收人手机号不可用")
		}
		return nil
	}
	validateProject := func(expectedID int) error {
		if intent.ProjectID != nil && *intent.ProjectID != expectedID {
			return ErrBadRequest("projectId与业务记录不匹配")
		}
		project, err := s.repo.Project.GetByID(ctx, expectedID)
		if err != nil {
			return ErrInternal("获取短信关联项目失败")
		}
		if project == nil {
			return ErrNotFound("短信关联项目不存在")
		}
		return nil
	}

	switch {
	case intent.ApplicationID != nil:
		if intent.NoticeType == nil || *intent.NoticeType == "" {
			return ErrBadRequest("申请短信必须提供noticeType")
		}
		noticeType := *intent.NoticeType
		if noticeType != "accepted" && noticeType != "rejected" && noticeType != "applicant_rejected" {
			return ErrBadRequest("申请短信noticeType无效")
		}
		app, err := s.repo.Application.GetByID(ctx, *intent.ApplicationID)
		if err != nil {
			return ErrInternal("获取申请记录失败")
		}
		if app == nil {
			return ErrNotFound("申请记录不存在")
		}
		expectedReceiverID := app.UserID
		if noticeType == "applicant_rejected" {
			if app.UserID != userID || app.ReviewerID == nil {
				return ErrForbidden("无权发送该申请短信")
			}
			expectedReceiverID = *app.ReviewerID
			initiated, checkErr := isApplicantInitiatedRejection(ctx, s.repo, app)
			if checkErr != nil {
				return ErrInternal("检查申请拒绝来源失败")
			}
			if !initiated {
				return ErrBadRequest("该申请不是申请人主动拒绝")
			}
		} else if app.ReviewerID == nil || *app.ReviewerID != userID {
			return ErrForbidden("无权发送该申请短信")
		}
		if (noticeType == "accepted" && app.Status != models.ApplicationStatusJoined) ||
			((noticeType == "rejected" || noticeType == "applicant_rejected") && app.Status != models.ApplicationStatusRejected) {
			return ErrBadRequest("申请状态与noticeType不匹配")
		}
		if err := validateReceiver(expectedReceiverID); err != nil {
			return err
		}
		return validateProject(app.ProjectID)

	case intent.MemberRemovalID != nil:
		if intent.NoticeType != nil && *intent.NoticeType != "removed" {
			return ErrBadRequest("成员移除短信不支持noticeType")
		}
		if s.repo.DB() == nil {
			return ErrInternal("数据库不可用")
		}
		var removal struct {
			UserID     int `db:"user_id"`
			ProjectID  int `db:"project_id"`
			OperatorID int `db:"operator_id"`
		}
		if err := s.repo.DB().GetContext(ctx, &removal,
			`SELECT user_id,project_id,operator_id FROM project_member_removal WHERE id=?`, *intent.MemberRemovalID); err != nil {
			return ErrNotFound("成员移除记录不存在")
		}
		if removal.OperatorID != userID {
			return ErrForbidden("无权发送该成员移除短信")
		}
		if err := validateReceiver(removal.UserID); err != nil {
			return err
		}
		return validateProject(removal.ProjectID)

	case intent.OliveBranchRecordID != nil:
		branch, err := s.repo.OliveBranch.GetByID(ctx, *intent.OliveBranchRecordID)
		if err != nil {
			return ErrInternal("获取橄榄枝记录失败")
		}
		if branch == nil {
			return ErrNotFound("橄榄枝记录不存在")
		}
		expectedReceiverID := branch.ReceiverID
		if intent.NoticeType == nil {
			if branch.SenderID != userID || branch.Status != models.OliveBranchStatusPending {
				return ErrBadRequest("橄榄枝状态或操作人不允许发送短信")
			}
		} else {
			noticeType := *intent.NoticeType
			if noticeType != "accepted" && noticeType != "rejected" && noticeType != "talent_rejected" {
				return ErrBadRequest("橄榄枝短信noticeType无效")
			}
			expectedStatus := models.OliveBranchStatusRejected
			if noticeType == "accepted" {
				expectedStatus = models.OliveBranchStatusAccepted
			}
			if noticeType == "talent_rejected" {
				expectedReceiverID = branch.SenderID
				if branch.ReceiverID != userID {
					return ErrForbidden("无权发送该橄榄枝结果短信")
				}
			} else if branch.SenderID != userID {
				return ErrForbidden("无权发送该橄榄枝结果短信")
			}
			if branch.Status != expectedStatus {
				return ErrBadRequest("橄榄枝状态与noticeType不匹配")
			}
		}
		if err := validateReceiver(expectedReceiverID); err != nil {
			return err
		}
		return validateProject(branch.RelatedProjectID)
	}
	return ErrBadRequest("短信通知必须关联业务记录")
}

// GetOrder retrieves an order with ownership check.
func (s *OrderService) GetOrder(ctx context.Context, userID, orderID int) (*models.Order, error) {
	order, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("[OrderService.GetOrder] repository error: %v", err)
		return nil, ErrInternal("获取订单详情失败")
	}
	if order == nil {
		return nil, ErrNotFound("订单不存在")
	}
	if order.UserID != userID {
		return nil, ErrForbidden("无权查看此订单")
	}
	return order, nil
}

// PaymentParams holds WeChat JSAPI payment parameters.
type PaymentParams = wechat.PaymentParams

// InitiatePayment validates the order and creates a WeChat prepay order.
func (s *OrderService) InitiatePayment(ctx context.Context, userID int, openID string, orderID int) (*PaymentParams, error) {
	if openID == "" {
		return nil, ErrBadRequest("无法获取用户OpenID")
	}

	order, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("[OrderService.InitiatePayment] repository error getting order: %v", err)
		return nil, ErrInternal("获取订单详情失败")
	}
	if order == nil {
		return nil, ErrNotFound("订单不存在")
	}
	if order.UserID != userID {
		return nil, ErrForbidden("无权操作此订单")
	}
	if order.Status != models.OrderStatusPending {
		return nil, ErrBadRequest("订单状态不允许支付")
	}
	intent, err := order.ParseDeliveryIntent()
	if err != nil {
		return nil, ErrBadRequest("订单交付信息无效")
	}
	if intent != nil {
		product, productErr := s.repo.Product.GetByID(ctx, order.ProductID)
		if productErr != nil {
			return nil, ErrInternal("获取商品信息失败")
		}
		if product == nil {
			return nil, ErrNotFound("商品不存在")
		}
		if validateErr := s.validateDeliveryIntent(ctx, userID, product, intent); validateErr != nil {
			return nil, validateErr
		}
	}

	if s.payInitErr != nil {
		log.Printf("[OrderService.InitiatePayment] wechat pay init error: %v", s.payInitErr)
		return nil, ErrInternal("支付配置错误: " + s.payInitErr.Error())
	}
	if s.payClient == nil {
		log.Printf("[OrderService.InitiatePayment] pay client is nil")
		return nil, ErrInternal("初始化支付客户端失败")
	}

	description := "快组校园商品购买"
	if order.ProductName != nil {
		description = *order.ProductName
	}

	outTradeNo := wechat.GenerateOutTradeNo(order.ID)
	amountCents := int(order.ActualPaid * 100)

	paymentParams, err := s.payClient.CreatePrepayOrderWithPayment(
		ctx,
		outTradeNo,
		description,
		openID,
		amountCents,
	)
	if err != nil {
		log.Printf("[OrderService.InitiatePayment] wechat API error: %v", err)
		return nil, ErrInternal("创建支付订单失败: " + err.Error())
	}

	return paymentParams, nil
}

// InitiateVirtualPayment signs an official wx.requestVirtualPayment request for in-scope virtual products.
func (s *OrderService) InitiateVirtualPayment(ctx context.Context, userID int, openID, loginCode string, orderID int) (*wechat.VirtualPaymentParams, error) {
	if openID == "" {
		return nil, ErrBadRequest("无法获取用户OpenID")
	}
	if strings.TrimSpace(loginCode) == "" {
		return nil, ErrBadRequest("缺少微信登录凭证")
	}
	order, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		return nil, ErrInternal("获取订单详情失败")
	}
	if order == nil {
		return nil, ErrNotFound("订单不存在")
	}
	if order.UserID != userID {
		return nil, ErrForbidden("无权操作此订单")
	}
	if order.Status != models.OrderStatusPending {
		return nil, ErrBadRequest("订单状态不允许支付")
	}
	product, err := s.repo.Product.GetByID(ctx, order.ProductID)
	if err != nil || product == nil {
		return nil, ErrInternal("获取商品信息失败")
	}
	switch product.ID {
	case 1, 2, 7, 9, 12:
	default:
		return nil, ErrBadRequest("该商品不属于本次虚拟支付范围")
	}
	intent, err := order.ParseDeliveryIntent()
	if err != nil {
		return nil, ErrBadRequest("订单交付信息无效")
	}
	if intent != nil {
		if err := s.validateDeliveryIntent(ctx, userID, product, intent); err != nil {
			return nil, err
		}
	}
	if s.virtualPayInitErr != nil {
		log.Printf("[OrderService.InitiateVirtualPayment] config error: %v", s.virtualPayInitErr)
		return nil, ErrInternal("虚拟支付配置错误")
	}
	if s.wxClient == nil || s.virtualPayConfig == nil {
		return nil, ErrInternal("虚拟支付未初始化")
	}
	wxSession, err := s.wxClient.Code2Session(strings.TrimSpace(loginCode))
	if err != nil {
		log.Printf("[OrderService.InitiateVirtualPayment] code2session error: %v", err)
		return nil, ErrBadRequest("微信登录凭证已失效，请重试")
	}
	if wxSession.OpenID != openID {
		return nil, ErrForbidden("微信登录身份不一致")
	}
	params, err := wechat.CreateVirtualPaymentParams(
		s.virtualPayConfig,
		wxSession.SessionKey,
		order.ID,
		order.CreatedAt,
		product.ID,
		// Sign with the immutable order snapshot. The catalog price may have
		// changed since an existing pending order was created, while the
		// callback deliberately validates against this same snapshot.
		order.Price,
		order.Quantity,
	)
	if err != nil {
		log.Printf("[OrderService.InitiateVirtualPayment] sign error: %v", err)
		return nil, ErrInternal("创建虚拟支付订单失败")
	}
	return params, nil
}

// CancelOrder cancels an unpaid order (status must be 0).
func (s *OrderService) CancelOrder(ctx context.Context, userID, orderID int) (*models.Order, error) {
	order, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("[OrderService.CancelOrder] repository error getting order: %v", err)
		return nil, ErrInternal("获取订单详情失败")
	}
	if order == nil {
		return nil, ErrNotFound("订单不存在")
	}
	if order.UserID != userID {
		return nil, ErrForbidden("无权操作此订单")
	}
	if order.Status != 0 {
		return nil, ErrBadRequest("订单状态不允许取消")
	}

	if err := IsValidStatus("order.status", models.OrderStatusCancelled); err != nil {
		return nil, err
	}

	if err := s.repo.Order.UpdateStatus(ctx, orderID, models.OrderStatusCancelled); err != nil {
		log.Printf("[OrderService.CancelOrder] repository error updating status: %v", err)
		return nil, ErrInternal("取消订单失败")
	}

	// Re-fetch to return updated order
	updated, err := s.repo.Order.GetByID(ctx, orderID)
	if err != nil {
		log.Printf("[OrderService.CancelOrder] repository error getting updated order: %v", err)
		return nil, ErrInternal("获取更新后的订单失败")
	}

	return updated, nil
}
