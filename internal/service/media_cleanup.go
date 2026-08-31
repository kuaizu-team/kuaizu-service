package service

import (
	"context"
	"log"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

const (
	mediaUnattachedRetention = 24 * time.Hour
	mediaCleanupClaimTimeout = time.Hour
	mediaCleanupInterval     = 6 * time.Hour
	mediaCleanupBatchSize    = 100
)

// StartMediaCleanupScheduler removes uploads that were never attached and
// retries OSS cleanup records left by interrupted or failed deletions.
func StartMediaCleanupScheduler(ctx context.Context, media *repository.MediaRepository, commons *CommonsService) {
	go func() {
		runMediaCleanup(ctx, media, commons, time.Now())
		ticker := time.NewTicker(mediaCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				runMediaCleanup(ctx, media, commons, now)
			}
		}
	}()
}

func runMediaCleanup(ctx context.Context, media *repository.MediaRepository, commons *CommonsService, now time.Time) {
	if media == nil || commons == nil {
		return
	}
	for {
		keys, err := media.ClaimCleanupBatch(ctx, now.Add(-mediaUnattachedRetention), now.Add(-mediaCleanupClaimTimeout), mediaCleanupBatchSize)
		if err != nil {
			if ctx.Err() == nil {
				log.Printf("[MediaCleanup] claim stale uploads failed: %v", err)
			}
			return
		}
		for _, key := range keys {
			if err := commons.DeleteFile(key); err != nil {
				log.Printf("[MediaCleanup] delete OSS object %s failed: %v", key, err)
				continue
			}
			if err := media.CompleteCleanup(ctx, key); err != nil {
				log.Printf("[MediaCleanup] complete cleanup record %s failed: %v", key, err)
			}
		}
		if len(keys) < mediaCleanupBatchSize {
			return
		}
	}
}
