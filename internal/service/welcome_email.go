package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/messagecenter"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

type WelcomeEmailService struct {
	repo             repository.WelcomeEmailDeliveryRepo
	messageCenter    *messagecenter.Client
	messageCenterErr error
}

func NewWelcomeEmailService(repo repository.WelcomeEmailDeliveryRepo, client *messagecenter.Client, initErr error) *WelcomeEmailService {
	return &WelcomeEmailService{repo: repo, messageCenter: client, messageCenterErr: initErr}
}

// Queue records this email-change event before starting background work.
// Retained for callers that do not already own a database transaction.
func (s *WelcomeEmailService) Queue(ctx context.Context, userID int, email, nickname string) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return
	}
	deliveryID, err := s.repo.Create(ctx, userID, email)
	if err != nil {
		log.Printf("[WelcomeEmail] create delivery failed user_id=%d: %v", userID, err)
		return
	}
	s.QueueDelivery(deliveryID, userID, email, nickname)
}

// QueueDelivery starts delivery for a history row that was committed together
// with the corresponding user email change.
func (s *WelcomeEmailService) QueueDelivery(deliveryID int64, userID int, email, nickname string) {
	email = strings.ToLower(strings.TrimSpace(email))
	if deliveryID <= 0 || email == "" {
		return
	}
	go s.send(deliveryID, userID, email, nickname)
}

// StartPendingRecovery retries committed deliveries that were left pending by
// a process interruption. The stable delivery trace ID keeps retries idempotent.
func (s *WelcomeEmailService) StartPendingRecovery(ctx context.Context) {
	go func() {
		s.recoverPending(ctx)
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.recoverPending(ctx)
			}
		}
	}()
}

func (s *WelcomeEmailService) recoverPending(ctx context.Context) {
	if s.messageCenterErr != nil || s.messageCenter == nil {
		log.Printf("[WelcomeEmail] pending recovery skipped: message center is unavailable")
		return
	}
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	staleBefore := time.Now().Add(-time.Minute)
	deliveries, err := s.repo.ListPendingBefore(queryCtx, staleBefore, 100)
	if err != nil {
		log.Printf("[WelcomeEmail] list pending deliveries failed: %v", err)
		return
	}
	queued := 0
	for _, delivery := range deliveries {
		if ctx.Err() != nil {
			return
		}
		claimed, err := s.repo.ClaimPending(queryCtx, delivery.ID, staleBefore)
		if err != nil {
			log.Printf("[WelcomeEmail] claim pending delivery id=%d failed: %v", delivery.ID, err)
			continue
		}
		if !claimed {
			continue
		}
		s.QueueDelivery(delivery.ID, delivery.UserID, delivery.Email, delivery.Nickname)
		queued++
	}
	if queued > 0 {
		log.Printf("[WelcomeEmail] queued %d pending deliveries for recovery", queued)
	}
}
func (s *WelcomeEmailService) send(deliveryID int64, userID int, email, nickname string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if s.messageCenterErr != nil || s.messageCenter == nil {
		err := s.messageCenterErr
		if err == nil {
			err = fmt.Errorf("message center client is nil")
		}
		s.fail(ctx, deliveryID, email, err)
		return
	}

	result, err := s.messageCenter.SendWelcomeEmail(ctx, messagecenter.WelcomeEmailRequest{
		Email: email, Nickname: nickname, TraceID: welcomeEmailTraceID(deliveryID),
	})
	if err != nil {
		s.fail(ctx, deliveryID, email, err)
		return
	}
	if !result.Success {
		s.fail(ctx, deliveryID, email, fmt.Errorf("message center send failed: %s", result.ErrorMessage))
		return
	}
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dbCancel()
	if err := s.repo.MarkSent(dbCtx, deliveryID, result.TaskID); err != nil {
		log.Printf("[WelcomeEmail] mark sent failed user_id=%d: %v", userID, err)
	}
}

func welcomeEmailTraceID(deliveryID int64) string {
	return fmt.Sprintf("welcome-email:%d", deliveryID)
}
func (s *WelcomeEmailService) fail(ctx context.Context, deliveryID int64, email string, sendErr error) {
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	if err := s.repo.MarkFailed(ctx, deliveryID, sendErr.Error()); err != nil {
		log.Printf("[WelcomeEmail] send failed (%v), mark failed also failed: %v", sendErr, err)
		return
	}
	log.Printf("[WelcomeEmail] send failed email=%s: %v", email, sendErr)
}
