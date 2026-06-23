package service

import (
	"context"
	"log"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

const projectDeleteRetentionDays = 7

// StartProjectCleanupScheduler runs the expired deleting-project cleanup once at
// startup and then every local midnight.
func StartProjectCleanupScheduler(ctx context.Context, repo *repository.Repository) {
	go func() {
		runProjectCleanup(ctx, repo)

		for {
			wait := time.Until(nextLocalMidnight(time.Now()))
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				runProjectCleanup(ctx, repo)
			}
		}
	}()
}

func runProjectCleanup(ctx context.Context, repo *repository.Repository) {
	cutoff := time.Now().AddDate(0, 0, -projectDeleteRetentionDays)
	deleted, err := repo.PurgeDeletedProjectsBefore(ctx, cutoff)
	if err != nil {
		log.Printf("[ProjectCleanup] purge expired deleted projects failed: %v", err)
		return
	}
	if deleted > 0 {
		log.Printf("[ProjectCleanup] purged %d expired deleted projects", deleted)
	}
}

func nextLocalMidnight(now time.Time) time.Time {
	localNow := now.In(time.Local)
	return time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, 0, 0, 0, 0, time.Local)
}
