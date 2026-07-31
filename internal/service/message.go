package service

import (
	"context"
	"database/sql"
	"encoding/json"
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

type permanentSubscribeDeliveryError struct {
	err error
}

func (e permanentSubscribeDeliveryError) Error() string { return e.err.Error() }
func (e permanentSubscribeDeliveryError) Unwrap() error { return e.err }

var chinaStandardTime = time.FixedZone("Asia/Shanghai", 8*60*60)

func formatChinaTime(value time.Time) string {
	return value.In(chinaStandardTime).Format("15:04")
}

func NewMessageService(repo *repository.Repository, wxClient *wechat.Client) *MessageService {
	return &MessageService{
		repo:     repo,
		wxClient: wxClient,
	}
}

// SendSubscribeMsgByBizKey durably queues a WeChat subscription message using a
// business key. The page path is taken from the template config in the database.
func (s *MessageService) SendSubscribeMsgByBizKey(ctx context.Context, userID int, bizKey string, businessData map[string]string) error {
	return s.enqueueSubscribeMsg(ctx, userID, bizKey, businessData, "")
}

// SendSubscribeMsgByBizKeyWithPage durably queues a subscription message with a
// caller-provided page path. It is used when the target page depends on the entity.
func (s *MessageService) SendSubscribeMsgByBizKeyWithPage(ctx context.Context, userID int, bizKey string, businessData map[string]string, pagePath string) error {
	return s.enqueueSubscribeMsg(ctx, userID, bizKey, businessData, pagePath)
}

func (s *MessageService) SendCollaborationScoreUpdateNotification(
	ctx context.Context,
	userID int,
	score float64,
	updatedAt time.Time,
) error {
	return s.SendSubscribeMsgByBizKey(ctx, userID, models.MsgBizKeyCollaborationScore, map[string]string{
		"score":      strconv.FormatFloat(score, 'f', -1, 64),
		"updated_at": formatChinaTime(updatedAt),
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
	if err := message.SendCollaborationScoreUpdateNotification(context.WithoutCancel(ctx), userID, score, updatedAt); err != nil {
		log.Printf("[%s] queue collaboration score notification failed (non-fatal), user_id=%d: %v", source, userID, err)
	}
}

// sendSubscribeMsg is the shared implementation. When pageOverride is non-empty it
// takes precedence over the page_path stored in msg_template_config.
func (s *MessageService) enqueueSubscribeMsg(ctx context.Context, userID int, bizKey string, businessData map[string]string, pageOverride string) error {
	payload, err := json.Marshal(businessData)
	if err != nil {
		return fmt.Errorf("marshal subscribe business data: %w", err)
	}
	var pagePath *string
	if pageOverride != "" {
		pagePath = &pageOverride
	}
	deliveryID, err := s.repo.WxSubscribeDelivery.Create(ctx, &models.WxSubscribeDelivery{
		UserID:       userID,
		BizKey:       bizKey,
		BusinessData: string(payload),
		PagePath:     pagePath,
		Status:       models.WxSubscribeDeliveryPending,
	})
	if err != nil {
		return fmt.Errorf("queue subscribe message: %w", err)
	}
	go s.processSubscribeDelivery(deliveryID)
	return nil
}

func (s *MessageService) deliverSubscribeMsg(ctx context.Context, userID int, bizKey string, businessData map[string]string, pageOverride string) (string, error) {
	// 1. Get user openid
	user, err := s.repo.User.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", permanentSubscribeDeliveryError{err: fmt.Errorf("user not found")}
		}
		return "", fmt.Errorf("get user: %w", err)
	}
	if user == nil || user.OpenID == "" {
		return "", permanentSubscribeDeliveryError{err: fmt.Errorf("user not found or has no openid")}
	}

	// 2. Get template config
	config, err := s.repo.MsgTemplate.GetByBizKey(ctx, bizKey)
	if err != nil {
		log.Printf("[MessageService.sendSubscribeMsg] error getting config for %s: %v", bizKey, err)
		if errors.Is(err, sql.ErrNoRows) {
			return "", permanentSubscribeDeliveryError{err: fmt.Errorf("template %s not found or disabled", bizKey)}
		}
		return "", fmt.Errorf("get template config: %w", err)
	}

	// 3. Determine page path: caller override takes precedence over DB value.
	// The local row mirrors only the last client callback; WeChat remains the
	// authoritative source for whether this delivery is currently allowed.
	page := pageOverride
	if page == "" && config.PagePath != nil {
		page = *config.PagePath
	}

	err = s.wxClient.SendByConfigContext(ctx, user.OpenID, config.TemplateID, config.ContentJSON, businessData, page)
	if err != nil {
		// 5. Sync state if user rejected on WeChat side
		var wxErr wechat.SubscribeMessageResponse
		if errors.As(err, &wxErr) {
			if wxErr.ErrCode == 43101 { // User refuse to accept
				log.Printf("[MessageService.sendSubscribeMsg] user %d rejected on WeChat, syncing local state", userID)
				if syncErr := s.repo.SubscribeConfig.UpsertWithHistory(ctx, &models.SubscribeConfig{
					UserID: userID,
					BizKey: bizKey,
					Status: models.SubscribeStatusReject,
				}, config.TemplateID, "reject", "wechat_send"); syncErr != nil {
					log.Printf("[MessageService.sendSubscribeMsg] sync rejection history failed: %v", syncErr)
				}
			}
		}

		log.Printf("[MessageService.sendSubscribeMsg] error sending message: %v", err)
		return config.TemplateID, fmt.Errorf("send message: %w", err)
	}

	return config.TemplateID, nil
}

func (s *MessageService) CheckSubscribeDeliverySchema(ctx context.Context) error {
	return s.repo.WxSubscribeDelivery.CheckSchema(ctx)
}

func (s *MessageService) StartSubscribeDeliveryRecovery(ctx context.Context) {
	go func() {
		s.recoverSubscribeDeliveries(ctx)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.recoverSubscribeDeliveries(ctx)
			}
		}
	}()
}

func (s *MessageService) recoverSubscribeDeliveries(ctx context.Context) {
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ids, err := s.repo.WxSubscribeDelivery.ListDue(queryCtx, time.Now().Add(-2*time.Minute), 100)
	if err != nil {
		log.Printf("[WxSubscribeDelivery] list due deliveries failed: %v", err)
		return
	}
	for _, id := range ids {
		go s.processSubscribeDelivery(id)
	}
}

func (s *MessageService) processSubscribeDelivery(deliveryID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	staleBefore := time.Now().Add(-2 * time.Minute)
	claimed, err := s.repo.WxSubscribeDelivery.Claim(ctx, deliveryID, staleBefore)
	if err != nil {
		log.Printf("[WxSubscribeDelivery] claim id=%d failed: %v", deliveryID, err)
		return
	}
	if !claimed {
		return
	}
	delivery, err := s.repo.WxSubscribeDelivery.GetByID(ctx, deliveryID)
	if err != nil {
		log.Printf("[WxSubscribeDelivery] load id=%d failed: %v", deliveryID, err)
		return
	}
	var businessData map[string]string
	if err := json.Unmarshal([]byte(delivery.BusinessData), &businessData); err != nil {
		s.finishSubscribeDelivery(delivery, "", permanentSubscribeDeliveryError{err: fmt.Errorf("decode business data: %w", err)})
		return
	}
	pagePath := ""
	if delivery.PagePath != nil {
		pagePath = *delivery.PagePath
	}
	templateID, sendErr := s.deliverSubscribeMsg(ctx, delivery.UserID, delivery.BizKey, businessData, pagePath)
	s.finishSubscribeDelivery(delivery, templateID, sendErr)
}

func (s *MessageService) finishSubscribeDelivery(delivery *models.WxSubscribeDelivery, templateID string, sendErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if sendErr == nil {
		if err := s.repo.WxSubscribeDelivery.MarkSent(ctx, delivery.ID, templateID); err != nil {
			log.Printf("[WxSubscribeDelivery] mark sent id=%d failed: %v", delivery.ID, err)
			return
		}
		log.Printf("[WxSubscribeDelivery] sent id=%d user_id=%d biz_key=%s", delivery.ID, delivery.UserID, delivery.BizKey)
		return
	}

	message := sendErr.Error()
	var wxErr wechat.SubscribeMessageResponse
	if errors.As(sendErr, &wxErr) && wxErr.ErrCode == 43101 {
		if err := s.repo.WxSubscribeDelivery.MarkSkipped(ctx, delivery.ID, templateID, wxErr.ErrCode, message); err != nil {
			log.Printf("[WxSubscribeDelivery] mark skipped id=%d failed: %v", delivery.ID, err)
		}
		return
	}

	var errCode *int
	permanent := false
	if errors.As(sendErr, &wxErr) {
		code := wxErr.ErrCode
		errCode = &code
		permanent = code == 40037 || code == 47003
	}
	var payloadErr wechat.SubscribePayloadError
	if errors.As(sendErr, &payloadErr) {
		permanent = true
	}
	var permanentErr permanentSubscribeDeliveryError
	if errors.As(sendErr, &permanentErr) {
		permanent = true
	}
	if permanent || delivery.AttemptCount >= 3 {
		if err := s.repo.WxSubscribeDelivery.MarkFailed(ctx, delivery.ID, templateID, errCode, message); err != nil {
			log.Printf("[WxSubscribeDelivery] mark failed id=%d failed: %v", delivery.ID, err)
		}
		return
	}
	delays := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute}
	delay := delays[delivery.AttemptCount-1]
	if err := s.repo.WxSubscribeDelivery.ScheduleRetry(ctx, delivery.ID, templateID, errCode, message, time.Now().Add(delay)); err != nil {
		log.Printf("[WxSubscribeDelivery] schedule retry id=%d failed: %v", delivery.ID, err)
	}
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
	var syncErrors []error
	for _, res := range syncResults {
		config, err := s.repo.MsgTemplate.GetByBizKey(ctx, res.BizKey)
		if err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("validate %s: %w", res.BizKey, err))
			continue
		}
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
		err = s.repo.SubscribeConfig.UpsertWithHistory(ctx, &models.SubscribeConfig{
			UserID: userID,
			BizKey: res.BizKey,
			Status: status,
		}, config.TemplateID, res.Result, "client_sync")
		if err != nil {
			log.Printf("[MessageService.SyncSubscribeStatus] upsert failed for %s: %v", res.BizKey, err)
			syncErrors = append(syncErrors, fmt.Errorf("sync %s: %w", res.BizKey, err))
			continue
		}
	}
	return errors.Join(syncErrors...)
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
	var enqueued int32
	var failed int32
	var skipped int32

	lastUserID := 0
	for {
		userIDs, err := s.repo.SubscribeConfig.ListAcceptedUserIDsByBizKey(ctx, models.MsgBizKeyVersionUpdate, pageSize, lastUserID)
		if err != nil {
			log.Printf("[VersionUpdateBroadcast] list users failed roadmap_id=%d after_user_id=%d: %v", roadmapID, lastUserID, err)
			break
		}
		if len(userIDs) == 0 {
			break
		}
		lastUserID = userIDs[len(userIDs)-1]

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
				atomic.AddInt32(&enqueued, 1)
			}()
		}
	}

	wg.Wait()
	log.Printf("[VersionUpdateBroadcast] done roadmap_id=%d candidates=%d enqueued=%d failed=%d skipped=%d",
		roadmapID, queued, enqueued, failed, skipped)
}

func truncateSubscribeThing(s string) string {
	runes := []rune(s)
	if len(runes) <= 20 {
		return s
	}
	return string(runes[:17]) + "..."
}
