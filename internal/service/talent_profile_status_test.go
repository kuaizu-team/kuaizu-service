package service

import (
	"testing"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

func TestResolveUpsertStatusNewProfile(t *testing.T) {
	svc := &TalentProfileService{}

	for _, tc := range []struct {
		name      string
		requested *api.TalentStatus
		want      int
	}{
		{name: "unspecified defaults to reviewing", want: models.TalentStatusReviewing},
		{name: "explicit private remains private", requested: talentStatusPtr(api.TalentStatus(models.TalentStatusPrivate)), want: models.TalentStatusPrivate},
		{name: "explicit online requires review", requested: talentStatusPtr(api.TalentStatus(models.TalentStatusOnline)), want: models.TalentStatusReviewing},
		{name: "explicit reviewing remains reviewing", requested: talentStatusPtr(api.TalentStatus(models.TalentStatusReviewing)), want: models.TalentStatusReviewing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.resolveUpsertStatus(tc.requested, nil)
			if err != nil {
				t.Fatalf("resolveUpsertStatus() error = %v", err)
			}
			if got == nil || *got != tc.want {
				t.Fatalf("resolveUpsertStatus() = %v, want %d", got, tc.want)
			}
		})
	}
}

func talentStatusPtr(status api.TalentStatus) *api.TalentStatus {
	return &status
}
