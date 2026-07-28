package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/kuaizu-team/kuaizu-service/internal/wechat"
)

// MessageService handles sending notifications (WeChat, etc.)
type MessageService struct {
	repo     *repository.Repository
	wxClient *wechat.Client
}

type VersionUpdateBroadcastResult struct {
	RoadmapID int    `json:"roadmapId"`
	Status    string `json:"status"`
}

var chinaStandardTime = time.FixedZone("Asia/Shanghai", 8*60*60)

func formatChinaMinute(value time.Time) string {
	return value.In(chinaStandardTime).Format("2006-01-02 15:04")
}

func NewMessageService(repo *repository.Repository, wxClient *wechat.Client) *MessageService {
	return &MessageService{
		repo:     repo,
		wxClient: wxClient,
	}
}

// SendSubscribeMsgByBizKey sends a WeChat subscription message using a business key.
// The page path is taken from the template config in the database.
func (s *MessageService) SendSubscribeMsgByBizKey(ctx context.Context, userID int, bizKey string, businessData map[string]string) error {
	return s.sendSubscribeMsg(ctx, userID, bizKey, businessData, "")
}

// SendSubscribeMsgByBizKeyWithPage sends a subscription message with a caller-provided
// page path. It is used when the target page depends on the concrete business entity.
func (s *MessageService) SendSubscribeMsgByBizKeyWithPage(ctx context.Context, userID int, bizKey string, businessData map[string]string, pagePath string) error {
	return s.sendSubscribeMsg(ctx, userID, bizKey, businessData, pagePath)
}

func (s *MessageService) SendCollaborationScoreUpdateNotification(
	ctx context.Context,
	userID int,
	score float64,
	updatedAt time.Time,
) error {
	return s.SendSubscribeMsgByBizKey(ctx, userID, models.MsgBizKeyCollaborationScore, map[string]string{
		"score":      strconv.FormatFloat(score, 'f', -1, 64),
		"updated_at": formatChinaMinute(updatedAt),
		"remark":     "请点击查看详情",
	})
}

func SendCollaborationScoreUpdateNotificationAsync(
	ctx context.Context,
	message *MessageService,
	userID int,
	score float64,
	updatedAt time.Time,
	source string,
) {
	if message == nil {
		return
	}
	go func(asyncCtx context.Context) {
		if err := message.SendCollaborationScoreUpdateNotification(asyncCtx, userID, score, updatedAt); err != nil {
			log.Printf("[%s] send collaboration score notification failed (non-fatal), user_id=%d: %v", source, userID, err)
		}
	}(context.WithoutCancel(ctx))
}

// sendSubscribeMsg is the shared implementation. When pageOverride is non-empty it
// takes precedence over the page_path stored in msg_template_config.
func (s *MessageService) sendSubscribeMsg(ctx context.Context, userID int, bizKey string, businessData map[string]string, pageOverride string) error {
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
		log.Printf("[MessageService.sendSubscribeMsg] error getting config for %s: %v", bizKey, err)
		return fmt.Errorf("get template config: %w", err)
	}

	// 3. Check local subscription status (Mirror)
	localSub, err := s.repo.SubscribeConfig.GetByUserIDAndBizKey(ctx, userID, bizKey)
	if err != nil {
		log.Printf("[MessageService.sendSubscribeMsg] check local sub error: %v", err)
	}
	if localSub != nil && localSub.Status == models.SubscribeStatusReject {
		log.Printf("[MessageService.sendSubscribeMsg] user %d rejected %s, skipping", userID, bizKey)
		return nil
	}

	// 4. Determine page path: caller override takes precedence over DB value
	page := pageOverride
	if page == "" && config.PagePath != nil {
		page = *config.PagePath
	}

	err = s.wxClient.SendByConfig(user.OpenID, config.TemplateID, config.ContentJSON, businessData, page)
	if err != nil {
		// 5. Sync state if user rejected on WeChat side
		var wxErr wechat.SubscribeMessageResponse
		if errors.As(err, &wxErr) {
			if wxErr.ErrCode == 43101 { // User refuse to accept
				log.Printf("[MessageService.sendSubscribeMsg] user %d rejected on WeChat, syncing local state", userID)
				_ = s.repo.SubscribeConfig.Upsert(ctx, &models.SubscribeConfig{
					UserID: userID,
					BizKey: bizKey,
					Status: models.SubscribeStatusReject,
				})
			}
		}

		log.Printf("[MessageService.sendSubscribeMsg] error sending message: %v", err)
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

func (s *MessageService) StartVersionUpdateBroadcast(ctx context.Context, roadmapID int) (*VersionUpdateBroadcastResult, error) {
	item, err := s.resolveVersionUpdateRoadmap(ctx, roadmapID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrNotFound("roadmap item not found")
	}

	result := &VersionUpdateBroadcastResult{RoadmapID: item.ID, Status: "started"}
	title := truncateSubscribeThing(item.Title)
	content := truncateSubscribeThing(item.Content)

	go s.broadcastVersionUpdate(context.Background(), item.ID, title, content)
	return result, nil
}

func (s *MessageService) resolveVersionUpdateRoadmap(ctx context.Context, roadmapID int) (*models.Roadmap, error) {
	if roadmapID > 0 {
		return s.repo.Roadmap.AdminGetByID(ctx, roadmapID)
	}
	return s.repo.Roadmap.Latest(ctx)
}

func (s *MessageService) broadcastVersionUpdate(ctx context.Context, roadmapID int, title string, content string) {
	const pageSize = 500
	const concurrency = 5

	log.Printf("[VersionUpdateBroadcast] start roadmap_id=%d", roadmapID)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var queued int32
	var sent int32
	var failed int32
	var skipped int32

	for offset := 0; ; offset += pageSize {
		userIDs, err := s.repo.SubscribeConfig.ListAcceptedUserIDsByBizKey(ctx, models.MsgBizKeyVersionUpdate, pageSize, offset)
		if err != nil {
			log.Printf("[VersionUpdateBroadcast] list users failed roadmap_id=%d offset=%d: %v", roadmapID, offset, err)
			break
		}
		if len(userIDs) == 0 {
			break
		}

		atomic.AddInt32(&queued, int32(len(userIDs)))
		for _, userID := range userIDs {
			uid := userID
			sem <- struct{}{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()

				callCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
				defer cancel()
				err := s.SendSubscribeMsgByBizKey(callCtx, uid, models.MsgBizKeyVersionUpdate, map[string]string{
					"title":   title,
					"content": content,
					"remark":  "点击查看详情",
				})
				if err != nil {
					log.Printf("[VersionUpdateBroadcast] send failed roadmap_id=%d user_id=%d: %v", roadmapID, uid, err)
					atomic.AddInt32(&failed, 1)
					return
				}
				atomic.AddInt32(&sent, 1)
			}()
		}
	}

	wg.Wait()
	log.Printf("[VersionUpdateBroadcast] done roadmap_id=%d queued=%d sent=%d failed=%d skipped=%d",
		roadmapID, queued, sent, failed, skipped)
}

func truncateSubscribeThing(s string) string {
	runes := []rune(s)
	if len(runes) <= 20 {
		return s
	}
	return string(runes[:17]) + "..."
}
