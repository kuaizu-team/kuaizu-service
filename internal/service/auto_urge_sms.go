package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kuaizu-team/kuaizu-service/internal/messagecenter"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

type autoUrgeStore interface {
	Candidates(context.Context, int, int) ([]repository.AutoUrgeCandidate, error)
	Claim(context.Context, int, string) (bool, error)
	Recheck(context.Context, int) (*repository.AutoUrgeCandidate, error)
	Snapshot(context.Context, repository.AutoUrgeCandidate, string) error
	Finish(context.Context, int, string, repository.AutoUrgeResult) error
}
type autoUrgeSender interface {
	Send(context.Context, messagecenter.AdminSmsSendRequest) (*messagecenter.AdminSmsSendResponse, error)
}

type autoUrgeWorker struct {
	store  autoUrgeStore
	sender autoUrgeSender
}

func (w autoUrgeWorker) run(ctx context.Context) error {
	after := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		candidates, err := w.store.Candidates(ctx, after, 200)
		if err != nil {
			return err
		}
		if len(candidates) == 0 {
			return nil
		}
		for _, c := range candidates {
			after = c.UserID
			if err := w.process(ctx, c.UserID); err != nil {
				return err
			}
		}
	}
}

func (w autoUrgeWorker) process(ctx context.Context, userID int) error {
	token := uuid.NewString()
	claimed, err := w.store.Claim(ctx, userID, token)
	if err != nil || !claimed {
		return err
	}
	// Persist outcomes even if shutdown or an outbound timeout cancels the scan.
	finish := func(result repository.AutoUrgeResult) error {
		saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result.ErrorCode = limitUrgeText(result.ErrorCode, 128)
		result.ErrorMessage = limitUrgeText(result.ErrorMessage, 500)
		return w.store.Finish(saveCtx, userID, token, result)
	}
	c, err := w.store.Recheck(ctx, userID)
	if err != nil {
		if saveErr := finish(repository.AutoUrgeResult{State: "failed", ErrorCode: "RECHECK_FAILED", ErrorMessage: "No SMS request dispatched; pending recheck failed"}); saveErr != nil {
			return saveErr
		}
		return err
	}
	if c == nil {
		return finish(repository.AutoUrgeResult{State: "ready", ErrorCode: "NO_LONGER_ELIGIBLE"})
	}
	if err := w.store.Snapshot(ctx, *c, token); err != nil {
		// No external call occurred. A failed state write leaves the claim blocked.
		_ = finish(repository.AutoUrgeResult{State: "failed", ErrorCode: "SNAPSHOT_FAILED", ErrorMessage: "No SMS request dispatched"})
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = finish(repository.AutoUrgeResult{State: "failed", ErrorCode: "CANCELLED_BEFORE_SEND"})
		return err
	}
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	response, sendErr := w.sender.Send(sendCtx, messagecenter.AdminSmsSendRequest{
		TemplateKey: "URGE_PROCESS", UserID: userID, Variables: map[string]interface{}{"nickname": c.Nickname},
	})
	cancel()
	result := classifyAutoUrgeResult(response, sendErr)
	if err := finish(result); err != nil {
		return fmt.Errorf("save automatic SMS result for user %d: %w", userID, err)
	}
	log.Printf("[AutoUrgeSMS] user_id=%d result=%s", userID, result.State)
	return nil
}

func classifyAutoUrgeResult(response *messagecenter.AdminSmsSendResponse, err error) repository.AutoUrgeResult {
	result := repository.AutoUrgeResult{State: "unknown"}
	if err != nil {
		result.ErrorCode = "OUTCOME_UNKNOWN"
		result.ErrorMessage = "Message center request failed or timed out; review its send record before retrying"
		return result
	}
	if response == nil {
		result.ErrorCode = "EMPTY_RESPONSE"
		return result
	}
	if response.RecordID > 0 {
		id := response.RecordID
		result.RecordID = &id
	}
	result.ErrorCode = response.ErrorCode
	result.ErrorMessage = response.ErrorMessage
	if response.Success {
		result.State = "sent"
		return result
	}
	// Only explicit rejections are retryable; ambiguous provider errors stay blocked.
	switch strings.ToLower(strings.TrimSpace(response.ErrorCode)) {
	case "isv.mobile_number_illegal", "isv.business_limit_control", "isv.amount_not_enough",
		"isv.sms_signature_illegal", "isv.sms_template_illegal", "isv.template_missing_parameters":
		result.State = "failed"
	}
	return result
}
func limitUrgeText(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func nextAutoUrgeScan(now time.Time) time.Time {
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	local := now.In(zone)
	next := time.Date(local.Year(), local.Month(), local.Day(), 10, 0, 0, 0, zone)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// Disabled by default. The operator must manually apply the provided migration
// and opt in. Never runs an immediate startup sweep or changes manual SMS logic.
func StartAutoUrgeSmsScheduler(ctx context.Context, repo *repository.Repository, sms *AdminSmsService) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("AUTO_URGE_SMS_ENABLED")), "true") {
		return
	}
	store := repository.NewAutoUrgeRepository(repo.DB())
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := store.CheckSchema(checkCtx)
	cancel()
	if err != nil {
		log.Printf("[AutoUrgeSMS] disabled: apply migration_auto_urge_sms_state.sql first: %v", err)
		return
	}
	go func() {
		for {
			timer := time.NewTimer(time.Until(nextAutoUrgeScan(time.Now())))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			// Configuration failures occur before any user claim and can recover next day.
			if _, err := sms.resolveMessageCenter(); err != nil {
				log.Printf("[AutoUrgeSMS] message center unavailable: %v", err)
				continue
			}
			scanCtx, cancel := context.WithTimeout(ctx, 2*time.Hour)
			err := (autoUrgeWorker{store: store, sender: sms}).run(scanCtx)
			cancel()
			if err != nil {
				log.Printf("[AutoUrgeSMS] scan stopped: %v", err)
			}
		}
	}()
}
