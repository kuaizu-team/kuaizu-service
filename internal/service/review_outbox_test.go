package service

import (
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"testing"
)

func TestReviewStateAndNotificationRollbackTogether(t *testing.T) {
	for _, kind := range []string{"project", "talent"} {
		for _, conflict := range []bool{false, true} {
			t.Run(kind+map[bool]string{true: " conflict", false: " outbox failure"}[conflict], func(t *testing.T) {
				raw, mock, err := sqlmock.New()
				if err != nil {
					t.Fatal(err)
				}
				defer raw.Close()
				repo := repository.New(sqlx.NewDb(raw, "mysql"))
				repo.User = projectUpdateUserRepoStub{}
				mock.ExpectBegin()
				affected := int64(1)
				if conflict {
					affected = 0
				}
				mock.ExpectExec("(?s)UPDATE " + map[string]string{"project": "project", "talent": "talent_profile"}[kind] + ".*WHERE id ?= ?\\? AND status ?= ?\\?").WillReturnResult(sqlmock.NewResult(0, affected))
				if !conflict {
					mock.ExpectExec("INSERT INTO wx_subscribe_delivery").WillReturnError(errors.New("outbox down"))
				}
				mock.ExpectRollback()
				if kind == "project" {
					err = (&ProjectService{repo: repo, message: NewMessageService(repo, nil)}).persistProjectReview(context.Background(), &models.Project{ID: 7, Status: 0, CreatorID: 8}, 1, nil, map[string]string{})
				} else {
					current := 2
					err = (&TalentProfileService{repo: repo, message: NewMessageService(repo, nil)}).persistTalentReview(context.Background(), &models.TalentProfile{ID: 7, UserID: 8, Status: &current}, 1, nil, "result", "remark")
				}
				if err == nil {
					t.Fatal("must not commit a partial review")
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
