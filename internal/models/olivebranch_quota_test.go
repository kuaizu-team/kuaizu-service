package models

import (
	"testing"
	"time"
)

func TestCalculateOliveBranchQuotaUsesAsiaShanghaiDate(t *testing.T) {
	used := 3
	paid := 1
	lastActiveDate := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)
	user := &User{
		FreeBranchUsedToday: &used,
		OliveBranchCount:    &paid,
		LastActiveDate:      &lastActiveDate,
	}

	now := time.Date(2026, 6, 3, 16, 30, 0, 0, time.UTC) // 2026-06-04 00:30 in Asia/Shanghai.
	quota := CalculateOliveBranchQuota(user, now)

	if quota.FreeBranchUsedToday != 3 || quota.FreeRemaining != 2 || quota.PaidBalance != 1 || quota.TotalRemaining != 3 {
		t.Fatalf("quota = %+v, want used=3 freeRemaining=2 paid=1 total=3", quota)
	}
}
