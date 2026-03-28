package service

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/trv3wood/kuaizu-server/internal/models"
	"github.com/trv3wood/kuaizu-server/internal/repository"
	"github.com/trv3wood/kuaizu-server/internal/wechat"
)

// MessageService handles sending notifications (WeChat, etc.)
type MessageService struct {
	repo     *repository.Repository
	wxClient *wechat.Client
}

func NewMessageService(repo *repository.Repository, wxClient *wechat.Client) *MessageService {
	return &MessageService{
		repo:     repo,
		wxClient: wxClient,
	}
}

// SendSubscribeMsgByBizKey sends a WeChat subscription message using a business key
func (s *MessageService) SendSubscribeMsgByBizKey(ctx context.Context, userID int, bizKey string, businessData map[string]string) error {
	// 1. Get user openid
	user, err := s.repo.User.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if user == nil || user.OpenID == "" {
		return fmt.Errorf("user not found or has no openid")
	}

	// 2. Get template config
	config, err := s.repo.MsgTemplate.GetByBizKey(ctx, bizKey)
	if err != nil {
		log.Printf("[MessageService.SendSubscribeMsgByBizKey] error getting config for %s: %v", bizKey, err)
		return fmt.Errorf("get template config: %w", err)
	}

	// 3. Check local subscription status (Mirror)
	localSub, err := s.repo.SubscribeConfig.GetByUserIDAndBizKey(ctx, userID, bizKey)
	if err != nil {
		log.Printf("[MessageService.SendSubscribeMsgByBizKey] check local sub error: %v", err)
	}
	if localSub != nil && localSub.Status == models.SubscribeStatusReject {
		log.Printf("[MessageService.SendSubscribeMsgByBizKey] user %d rejected %s, skipping", userID, bizKey)
		return nil
	}

	// 4. Send using client helper
	err = s.wxClient.SendByConfig(user.OpenID, config.TemplateID, config.ContentJSON, businessData)
	if err != nil {
		// 5. Sync state if user rejected on WeChat side
		var wxErr wechat.SubscribeMessageResponse
		if errors.As(err, &wxErr) {
			if wxErr.ErrCode == 43101 { // User refuse to accept
				log.Printf("[MessageService.SendSubscribeMsgByBizKey] user %d rejected on WeChat, syncing local state", userID)
				_ = s.repo.SubscribeConfig.Upsert(ctx, &models.SubscribeConfig{
					UserID: userID,
					BizKey: bizKey,
					Status: models.SubscribeStatusReject,
				})
			}
		}

		log.Printf("[MessageService.SendSubscribeMsgByBizKey] error sending message: %v", err)
		return fmt.Errorf("send message: %w", err)
	}

	return nil
}

// GetMsgTemplatesByBizKeys retrieves multiple message template configurations by their business keys
func (s *MessageService) GetMsgTemplatesByBizKeys(ctx context.Context, bizKeys []string) ([]models.MsgTemplateConfig, error) {
	configs, err := s.repo.MsgTemplate.GetByBizKeys(ctx, bizKeys)
	if err != nil {
		log.Printf("[MessageService.GetMsgTemplatesByBizKeys] error: %v", err)
		return nil, fmt.Errorf("get msg templates by biz_keys: %w", err)
	}
	return configs, nil
}

// SyncSubscribeStatus syncs user subscription status from frontend
type TemplateSyncResult struct {
	BizKey string
	Result string // accept, reject, ban
}

func (s *MessageService) SyncSubscribeStatus(ctx context.Context, userID int, syncResults []TemplateSyncResult) error {
	for _, res := range syncResults {
		// 1. Map result to status
		var status models.SubscribeStatus
		switch res.Result {
		case "accept":
			status = models.SubscribeStatusAccept
		case "reject":
			status = models.SubscribeStatusReject
		default:
			status = models.SubscribeStatusReject // treat ban or other as reject
		}

		// 2. Upsert
		err := s.repo.SubscribeConfig.Upsert(ctx, &models.SubscribeConfig{
			UserID: userID,
			BizKey: res.BizKey,
			Status: status,
		})
		if err != nil {
			log.Printf("[MessageService.SyncSubscribeStatus] upsert failed for %s: %v", res.BizKey, err)
		}
	}
	return nil
}
