package handler

import (
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"testing"
)

func TestProfileBadgesSumImmediateChildren(t *testing.T) {
	olive := repository.OliveBranchBadgeCounts{ReceivedPendingCount: 3, SentUnreadCount: 4}
	favorites := models.FavoriteViewState{ProjectCount: 8, TalentCount: 9}
	project := repository.ProfileProjectBadgeState{PendingApplicationCount: 2, HasStatusUnread: true}
	for _, counts := range []struct{ project, talent int }{{7, 5}, {0, 5}, {0, 0}} {
		got := aggregateProfileBadges(6, olive, favorites, repository.DashboardUnreadTotals{ProjectCount: counts.project, TalentCount: counts.talent}, project, true)
		if got.ProjectBadge != 2+counts.project || got.CardBadge != 6+counts.talent {
			t.Fatalf("child sum mismatch: %+v", got)
		}
		if got.TotalBadge != got.CardBadge+got.OliveBadge+got.ProjectBadge {
			t.Fatal("tab bar sum mismatch")
		}
		if got.OliveBadge != 7 || got.HomeBadge != 17 || !got.HasProjectStatusUnread {
			t.Fatal("unrelated badges changed")
		}
	}
	legacy := aggregateProfileBadges(6, olive, favorites, repository.DashboardUnreadTotals{ProjectCount: 7, TalentCount: 5}, project, false)
	if legacy.CardBadge != 6 || legacy.ProjectBadge != 9 || legacy.TotalBadge != 22 {
		t.Fatalf("legacy snapshot changed: %+v", legacy)
	}
}
