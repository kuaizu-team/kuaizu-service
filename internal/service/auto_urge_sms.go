package service

import (
	"context"
	"errors"
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
	Claim(context.Context, int, string) (*repository.AutoUrgeClaim, error)
	Recheck(context.Context, int) (*repository.AutoUrgeCandidate, error)
	Snapshot(context.Context, repository.AutoUrgeCandidate, repository.AutoUrgeClaim) error
	MarkDispatched(context.Context, repository.AutoUrgeClaim) error
	Finish(context.Context, repository.AutoUrgeClaim, repository.AutoUrgeResult) error
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
	var failures []error
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		candidates, err := w.store.Candidates(ctx, after, 200)
		if err != nil {
			return err
		}
		if len(candidates) == 0 {
			return errors.Join(failures...)
		}
		for _, c := range candidates {
			after = c.UserID
			if err := w.process(ctx, c.UserID); err != nil {
				failures = append(failures, fmt.Errorf("automatic SMS user %d: %w", c.UserID, err))
				log.Printf("[AutoUrgeSMS] user_id=%d failed: %v", c.UserID, err)
			}
		}
	}
}

func (w autoUrgeWorker) process(ctx context.Context, userID int) error {
	token := uuid.NewString()
	claim, err := w.store.Claim(ctx, userID, token)
	if err != nil || claim == nil {
		return err
	}
	// Persist outcomes even if shutdown or an outbound timeout cancels the scan.
	finish := func(result repository.AutoUrgeResult) error {
		saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result.ErrorCode = limitUrgeText(result.ErrorCode, 128)
		result.ErrorMessage = limitUrgeText(result.ErrorMessage, 500)
		return w.store.Finish(saveCtx, *claim, result)
	}
	if claim.ReconcileOnly {
		sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		response, sendErr := w.sender.Send(sendCtx, messagecenter.AdminSmsSendRequest{
			TemplateKey: "URGE_PROCESS", UserID: userID, RequestKey: claim.RequestKey, ReconcileOnly: true,
		})
		cancel()
		result := classifyAutoUrgeResult(response, sendErr)
		// NOT_DISPATCHED is meaningful only for the read-only lookup of this key.
		if sendErr == nil && response != nil && response.ErrorCode == "NOT_DISPATCHED" {
			result.State = "failed"
		}
		return finish(result)
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
	if err := w.store.Snapshot(ctx, *c, *claim); err != nil {
		// No external call occurred. A failed state write leaves the claim blocked.
		_ = finish(repository.AutoUrgeResult{State: "failed", ErrorCode: "SNAPSHOT_FAILED", ErrorMessage: "No SMS request dispatched"})
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = finish(repository.AutoUrgeResult{State: "failed", ErrorCode: "CANCELLED_BEFORE_SEND"})
		return err
	}
	if err := w.store.MarkDispatched(ctx, *claim); err != nil {
		_ = finish(repository.AutoUrgeResult{State: "failed", ErrorCode: "DISPATCH_FENCE_FAILED", ErrorMessage: "No SMS request dispatched"})
		return err
	}
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	response, sendErr := w.sender.Send(sendCtx, messagecenter.AdminSmsSendRequest{
		TemplateKey: "URGE_PROCESS", UserID: userID, RequestKey: claim.RequestKey, Variables: map[string]interface{}{"nickname": c.Nickname},
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
		"isv.sms_signature_illegal", "isv.sms_template_illegal", "isv.template_missing_parameters",
		"no_sms_provider", "user_not_found", "invalid_phone":
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

// Disabled by default. After schema validation, opt-in enables cycle tracking
// and the daily scan. A startup after 10:00 catches up with stable request keys.
func StartAutoUrgeSmsScheduler(ctx context.Context, repo *repository.Repository, sms *AdminSmsService) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("AUTO_URGE_SMS_ENABLED")), "true") {
		return
	}
	store := repo.AutoUrge
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := store.CheckSchema(checkCtx)
	cancel()
	if err != nil {
		log.Printf("[AutoUrgeSMS] disabled: apply the 20261006 SMS field migrations first: %v", err)
		return
	}
	store.EnableTracking()
	go func() {
		lastCompleted := ""
		for {
			timer := time.NewTimer(time.Until(autoUrgeScanDue(time.Now(), lastCompleted)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			// Configuration failures occur before claims; retry the scan after five minutes.
			if _, err := sms.resolveMessageCenter(); err != nil {
				log.Printf("[AutoUrgeSMS] message center unavailable: %v", err)
				if !waitAutoUrgeRetry(ctx) {
					return
				}
				continue
			}
			scanDay := autoUrgeLocalDay(time.Now())
			scanCtx, cancel := context.WithTimeout(ctx, 2*time.Hour)
			err := (autoUrgeWorker{store: store, sender: sms}).run(scanCtx)
			cancel()
			if err != nil {
				log.Printf("[AutoUrgeSMS] scan incomplete: %v", err)
				if !waitAutoUrgeRetry(ctx) {
					return
				}
			} else {
				lastCompleted = scanDay
			}
		}
	}()
}

// One completed scan per local day in this process. Restarts may rescan; the
// persisted per-cycle request key prevents another successful provider call.
func autoUrgeLocalDay(now time.Time) string {
	return now.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01-02")
}
func autoUrgeScanDue(now time.Time, lastCompleted string) time.Time {
	local := now.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	today := time.Date(local.Year(), local.Month(), local.Day(), 10, 0, 0, 0, local.Location())
	if !now.Before(today) && lastCompleted != autoUrgeLocalDay(now) {
		return now
	}
	return nextAutoUrgeScan(now)
}
func waitAutoUrgeRetry(ctx context.Context) bool {
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
