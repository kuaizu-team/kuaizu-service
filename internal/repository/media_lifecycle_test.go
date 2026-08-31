package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

func TestUpdateProjectRollsBackWhenImageValidationFails(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()

	repo := NewProjectRepository(sqlx.NewDb(raw, "sqlmock"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE project SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT object_key FROM media_upload").
		WithArgs("project-images/2026/08/31/not-owned.jpg", 17, MediaTypeProjectImage, "project", 42).
		WillReturnRows(sqlmock.NewRows([]string{"object_key"}))
	mock.ExpectRollback()

	keys := []string{"project-images/2026/08/31/not-owned.jpg"}
	_, err = repo.UpdateWithMetadata(context.Background(), &models.Project{ID: 42}, nil, nil, nil, nil, nil, nil, 17, &keys, true)
	if !errors.Is(err, ErrInvalidMedia) {
		t.Fatalf("error = %v, want ErrInvalidMedia", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertTalentProfileRollsBackWhenImageValidationFails(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()

	repo := NewTalentProfileRepository(sqlx.NewDb(raw, "sqlmock"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM talent_profile").
		WithArgs(17).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(91))
	mock.ExpectExec("UPDATE talent_profile SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT object_key FROM media_upload").
		WithArgs("talent-work-images/2026/08/31/not-owned.jpg", 17, MediaTypeTalentWork, "talent_profile", 91).
		WillReturnRows(sqlmock.NewRows([]string{"object_key"}))
	mock.ExpectRollback()

	profile := &models.TalentProfile{UserID: 17}
	_, err = repo.UpsertWithWorkImages(context.Background(), profile, 17, []string{"talent-work-images/2026/08/31/not-owned.jpg"})
	if !errors.Is(err, ErrInvalidMedia) {
		t.Fatalf("error = %v, want ErrInvalidMedia", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFinalizeMilestoneReviewCommitsBeforeReturningCleanupKeys(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()

	repo := NewMediaRepository(sqlx.NewDb(raw, "sqlmock"))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT object_key FROM project_milestone_evidence").
		WithArgs(55).
		WillReturnRows(sqlmock.NewRows([]string{"object_key"}).
			AddRow("milestone-evidence/one.jpg").
			AddRow("milestone-evidence/two.jpg"))
	mock.ExpectExec("UPDATE project_milestones SET certification_status").
		WithArgs(2, 55).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE media_upload SET attached_type").
		WithArgs(mediaAttachmentCleanup, 55).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("DELETE FROM project_milestone_evidence").
		WithArgs(55).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	keys, err := repo.FinalizeMilestoneReview(context.Background(), 55, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("keys = %#v, want two cleanup keys", keys)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimCleanupBatchClaimsUnattachedAndExpiredCleanupRecords(t *testing.T) {
	raw, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()

	repo := NewMediaRepository(sqlx.NewDb(raw, "sqlmock"))
	unattachedBefore := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	staleClaimBefore := time.Date(2026, 8, 31, 11, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT object_key FROM media_upload").
		WithArgs(unattachedBefore, mediaAttachmentCleanup, staleClaimBefore, 100).
		WillReturnRows(sqlmock.NewRows([]string{"object_key"}).
			AddRow("project-images/unattached.jpg").
			AddRow("project-images/retry.jpg"))
	for _, key := range []string{"project-images/unattached.jpg", "project-images/retry.jpg"} {
		mock.ExpectExec("UPDATE media_upload SET attached_type").
			WithArgs(mediaAttachmentCleanup, key, unattachedBefore, mediaAttachmentCleanup, staleClaimBefore).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()

	keys, err := repo.ClaimCleanupBatch(context.Background(), unattachedBefore, staleClaimBefore, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("keys = %#v, want two claimed keys", keys)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
