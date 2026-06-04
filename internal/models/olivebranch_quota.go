package models

import "time"

var oliveBranchQuotaLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type OliveBranchQuota struct {
	DailyFreeQuota      int
	FreeBranchUsedToday int
	FreeRemaining       int
	PaidBalance         int
	TotalRemaining      int
}

func CalculateOliveBranchQuota(user *User, now time.Time) OliveBranchQuota {
	freeBranchUsedToday := 0
	if user != nil && user.FreeBranchUsedToday != nil && isSameDate(user.LastActiveDate, now) {
		freeBranchUsedToday = *user.FreeBranchUsedToday
	}
	if freeBranchUsedToday < 0 {
		freeBranchUsedToday = 0
	}

	freeRemaining := OliveBranchDailyFreeQuota - freeBranchUsedToday
	if freeRemaining < 0 {
		freeRemaining = 0
	}

	paidBalance := 0
	if user != nil && user.OliveBranchCount != nil {
		paidBalance = *user.OliveBranchCount
	}
	if paidBalance < 0 {
		paidBalance = 0
	}

	return OliveBranchQuota{
		DailyFreeQuota:      OliveBranchDailyFreeQuota,
		FreeBranchUsedToday: freeBranchUsedToday,
		FreeRemaining:       freeRemaining,
		PaidBalance:         paidBalance,
		TotalRemaining:      freeRemaining + paidBalance,
	}
}

func isSameDate(date *time.Time, now time.Time) bool {
	if date == nil {
		return false
	}
	dateInLocation := date.In(oliveBranchQuotaLocation)
	nowInLocation := now.In(oliveBranchQuotaLocation)
	y1, m1, d1 := dateInLocation.Date()
	y2, m2, d2 := nowInLocation.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}
