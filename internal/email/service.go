package email

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

// Service 邮件推广服务
type Service struct {
	client           Client
	templateRenderer *TemplateRenderer
	userRepo         repository.UserRepo
	projectRepo      repository.ProjectRepo
	promotionRepo    repository.EmailPromotionRepo
}

// NewService 创建邮件服务
func NewService(
	client Client,
	baseURL string,
	userRepo repository.UserRepo,
	projectRepo repository.ProjectRepo,
	promotionRepo repository.EmailPromotionRepo,
) *Service {
	return &Service{
		client:           client,
		templateRenderer: NewTemplateRenderer(baseURL),
		userRepo:         userRepo,
		projectRepo:      projectRepo,
		promotionRepo:    promotionRepo,
	}
}

// NewServiceFromEnv 从环境变量创建邮件服务
func NewServiceFromEnv(
	userRepo repository.UserRepo,
	projectRepo repository.ProjectRepo,
	promotionRepo repository.EmailPromotionRepo,
) (*Service, error) {
	client, err := NewSMTPClientFromEnv()
	if err != nil {
		return nil, err
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "https://kuaizu.xyz"
	}

	return NewService(client, baseURL, userRepo, projectRepo, promotionRepo), nil
}

// SendPromotionEmails 发送推广邮件
// 这是一个长时间运行的操作，应该在 goroutine 中调用
func (s *Service) SendPromotionEmails(ctx context.Context, promotion *models.EmailPromotion) {
	// 1. 更新状态为发送中
	now := time.Now()
	promotion.Status = models.EmailPromotionStatusSending
	promotion.StartedAt = &now
	s.promotionRepo.Update(ctx, promotion)

	// 2. 获取项目信息
	project, err := s.projectRepo.GetByID(ctx, promotion.ProjectID)
	if err != nil || project == nil {
		errMsg := "项目不存在"
		promotion.Status = models.EmailPromotionStatusFailed
		promotion.ErrorMessage = &errMsg
		s.promotionRepo.Update(ctx, promotion)
		return
	}

	// 3. 查询发送对象 (带 LIMIT)
	recipients, err := s.userRepo.FindEmailRecipients(ctx, promotion.CreatorID, promotion.MaxRecipients)
	if err != nil {
		errMsg := err.Error()
		promotion.Status = models.EmailPromotionStatusFailed
		promotion.ErrorMessage = &errMsg
		s.promotionRepo.Update(ctx, promotion)
		return
	}

	// 4. 批量发送
	sentCount := 0
	for _, r := range recipients {
		// 生成退订 token
		unsubscribeToken := generateUnsubscribeTokenForEmail(r.ID)

		// 渲染邮件
		subject, body, err := s.templateRenderer.RenderProjectPromotion(project, r.Nickname, unsubscribeToken)
		if err != nil {
			continue
		}

		// 发送邮件
		if err := s.client.Send(r.Email, subject, body); err == nil {
			sentCount++
		}

		// 延迟发送，避免触发反垃圾机制
		time.Sleep(100 * time.Millisecond)
	}

	// 5. 更新完成状态
	completedAt := time.Now()
	promotion.TotalSent = sentCount
	promotion.Status = models.EmailPromotionStatusCompleted
	promotion.CompletedAt = &completedAt
	s.promotionRepo.Update(ctx, promotion)
}

// generateUnsubscribeTokenForEmail 为邮件生成退订token
func generateUnsubscribeTokenForEmail(userID int) string {
	timestamp := time.Now().Unix()
	data := fmt.Sprintf("%d:%d", userID, timestamp)

	secretKey := os.Getenv("JWT_SECRET")
	if secretKey == "" {
		secretKey = "kuaizu-default-secret-key"
	}

	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(data))
	sig := hex.EncodeToString(mac.Sum(nil))[:16]

	payload := data + ":" + sig
	return base64.URLEncoding.EncodeToString([]byte(payload))
}
