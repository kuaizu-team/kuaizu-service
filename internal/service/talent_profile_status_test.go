package service

import (
	"testing"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

func TestResolveUpsertStatusNewProfileDefaultsToReviewing(t *testing.T) {
	svc := &TalentProfileService{}

	for _, requested := range []*api.TalentStatus{
		nil,
		talentStatusPtr(api.TalentStatus(models.TalentStatusPrivate)),
		talentStatusPtr(api.TalentStatus(models.TalentStatusOnline)),
	} {
		got, err := svc.resolveUpsertStatus(requested, nil)
		if err != nil {
			t.Fatalf("resolveUpsertStatus() error = %v", err)
		}
		if got == nil || *got != models.TalentStatusReviewing {
			t.Fatalf("resolveUpsertStatus() = %v, want %d", got, models.TalentStatusReviewing)
		}
	}
}

func talentStatusPtr(status api.TalentStatus) *api.TalentStatus {
	return &status
}
