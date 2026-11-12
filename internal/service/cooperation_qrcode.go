package service

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

const cooperationQRValidity = 7 * 24 * time.Hour

func (s *CampusCooperationService) prepareQRContacts(ctx context.Context, tx *sqlx.Tx, id int64, contacts, old []models.CooperationContact) error {
	now := time.Now()
	previousQR := map[string]models.CooperationContact{}
	for _, contact := range old {
		if contact.Type == "wechat_group" {
			previousQR[contact.Value] = contact
		}
	}
	for i := range contacts {
		contact := &contacts[i]
		contact.ImageURL = ""
		contact.Expired = false
		if contact.Type != "wechat_group" {
			contact.ExpiresAt = nil
			continue
		}
		// Already expired persisted entries may be kept while other content is edited,
		// even after the cleanup worker has removed their ownership rows.
		if previous, ok := previousQR[contact.Value]; ok && previous.ExpiresAt != nil && !now.Before(*previous.ExpiresAt) {
			contact.ExpiresAt = previous.ExpiresAt
			continue
		}
		var expires sql.NullTime
		if err := tx.GetContext(ctx, &expires, `SELECT expires_at FROM campus_cooperation_image WHERE object_key=? AND cooperation_id=? FOR UPDATE`, contact.Value, id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrBadRequest("二维码不存在或不属于本校合作")
			}
			return err
		}
		if !expires.Valid || !now.Before(expires.Time) {
			return ErrBadRequest("微信群二维码已过期，请重新上传或移除该项")
		}
		expiry := expires.Time
		contact.ExpiresAt = &expiry
	}
	retained := map[string]bool{}
	for _, contact := range contacts {
		if contact.Type == "wechat_group" {
			retained[contact.Value] = true
		}
	}
	for _, contact := range old {
		if contact.Type != "wechat_group" || retained[contact.Value] {
			continue
		}
		// Revoke before releasing the cooperation lock, so stale forms cannot attach
		// an old QR while its OSS object is being removed after commit.
		if _, err := tx.ExecContext(ctx, `UPDATE campus_cooperation_image SET expires_at=LEAST(COALESCE(expires_at,NOW()),NOW()) WHERE object_key=? AND cooperation_id=?`, contact.Value, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *CampusCooperationService) removeReplacedQRCodes(ctx context.Context, id int64, old, contacts []models.CooperationContact) {
	if s.deleteQRFile == nil {
		return
	}
	retained := map[string]bool{}
	for _, contact := range contacts {
		if contact.Type == "wechat_group" {
			retained[contact.Value] = true
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for _, contact := range old {
		if contact.Type != "wechat_group" || retained[contact.Value] {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		if err := s.deleteQRFile(ctx, contact.Value); err != nil {
			log.Printf("[CooperationQR] replacement cleanup deferred cooperation=%d", id)
			continue
		}
		if _, err := s.repo.DB().ExecContext(ctx, `DELETE FROM campus_cooperation_image WHERE object_key=? AND cooperation_id=? AND expires_at<=NOW()`, contact.Value, id); err != nil {
			log.Printf("[CooperationQR] replacement cleanup bookkeeping failed cooperation=%d", id)
		}
	}
}

// One bounded sweep every five minutes; no retries within a sweep. Expiration
// in API responses is enforced independently of OSS deletion or worker uptime.
func (s *CampusCooperationService) StartQRCodeCleanup(ctx context.Context) {
	go func() {
		s.cleanupExpiredQRCodes(ctx)
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.cleanupExpiredQRCodes(ctx)
			}
		}
	}()
}

func (s *CampusCooperationService) cleanupExpiredQRCodes(root context.Context) {
	if s.deleteQRFile == nil {
		return
	}
	ctx, cancel := context.WithTimeout(root, 90*time.Second)
	defer cancel()
	var keys []string
	if err := s.repo.DB().SelectContext(ctx, &keys, `SELECT object_key FROM campus_cooperation_image WHERE expires_at<=? AND object_key LIKE ? ORDER BY expires_at,object_key LIMIT 100`, time.Now(), models.CooperationQRPrefix+"%"); err != nil {
		if ctx.Err() == nil {
			log.Printf("[CooperationQR] expiry scan failed; check QR migration: %v", err)
		}
		return
	}
	for _, key := range keys {
		if ctx.Err() != nil {
			return
		}
		deleteCtx, stop := context.WithTimeout(ctx, 3*time.Second)
		err := s.deleteQRFile(deleteCtx, key)
		stop()
		if err != nil {
			log.Printf("[CooperationQR] expired object deletion deferred")
			continue
		}
		if _, err = s.repo.DB().ExecContext(ctx, `DELETE FROM campus_cooperation_image WHERE object_key=? AND expires_at<=?`, key, time.Now()); err != nil {
			log.Printf("[CooperationQR] expired object bookkeeping failed")
			return
		}
	}
}
