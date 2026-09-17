package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/require"
)

func timelineTestService(t *testing.T) (*ProjectService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, mock.ExpectationsWereMet()); _ = db.Close() })
	return &ProjectService{repo: repository.New(sqlx.NewDb(db, "sqlmock"))}, mock
}

func TestMemberTimelineRejectsNonMembers(t *testing.T) {
	for _, action := range []string{"list", "create", "update", "delete"} {
		t.Run(action, func(t *testing.T) {
			s, mock := timelineTestService(t)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT id FROM project_members").WithArgs(42, 7).WillReturnRows(sqlmock.NewRows([]string{"id"}))
			mock.ExpectRollback()
			var err error
			switch action {
			case "list":
				_, err = s.ListMemberTimeline(context.Background(), 42, 7, 0)
			case "create":
				_, err = s.SaveMemberTimeline(context.Background(), 42, 7, 0, MemberTimelineInput{Title: "进度"})
			case "update":
				_, err = s.SaveMemberTimeline(context.Background(), 42, 7, 9, MemberTimelineInput{Title: "进度"})
			case "delete":
				err = s.DeleteMemberTimeline(context.Background(), 42, 7, 9)
			}
			var business *ServiceError
			require.ErrorAs(t, err, &business)
			require.Equal(t, ErrCodeForbidden, business.Code)
		})
	}
}

func TestMemberTimelineCannotModifyAnotherOwnerOrProject(t *testing.T) {
	for _, action := range []string{"update", "delete"} {
		t.Run(action, func(t *testing.T) {
			s, mock := timelineTestService(t)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT id FROM project_members").WithArgs(42, 7).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(100))
			if action == "update" {
				mock.ExpectQuery("SELECT related_members FROM project_member_timeline WHERE id=\\? AND project_id=\\? AND user_id=\\?").WithArgs(9, 42, 7).WillReturnRows(sqlmock.NewRows([]string{"id"}))
			} else {
				mock.ExpectExec("DELETE FROM project_member_timeline WHERE id=\\? AND project_id=\\? AND user_id=\\?").WithArgs(9, 42, 7).WillReturnResult(sqlmock.NewResult(0, 0))
			}
			mock.ExpectRollback()
			var err error
			if action == "update" {
				_, err = s.SaveMemberTimeline(context.Background(), 42, 7, 9, MemberTimelineInput{Title: "进度"})
			} else {
				err = s.DeleteMemberTimeline(context.Background(), 42, 7, 9)
			}
			var business *ServiceError
			require.ErrorAs(t, err, &business)
			require.Equal(t, ErrCodeForbidden, business.Code)
		})
	}
}

func TestMemberTimelineRejoinReadsHistoryUsingUserID(t *testing.T) {
	s, mock := timelineTestService(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	// New membership ID deliberately differs from any historical membership.
	mock.ExpectQuery("SELECT id FROM project_members").WithArgs(42, 7).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(999))
	mock.ExpectQuery("SELECT n.\\* FROM project_member_timeline n").WithArgs(42, 7, 7).
		WillReturnRows(sqlmock.NewRows([]string{"id", "project_id", "user_id", "title", "detail", "related_members", "created_at", "updated_at"}).
			AddRow(1, 42, 7, "历史", "详情", `[{"userId":8,"nickname":"伙伴"}]`, created, created))
	mock.ExpectQuery("SELECT user_id FROM project_members").WithArgs(42).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(7))
	mock.ExpectCommit()
	nodes, err := s.ListMemberTimeline(context.Background(), 42, 7, 7)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.Equal(t, "伙伴", nodes[0].RelatedMembers[0].Nickname)
	require.True(t, nodes[0].RelatedMembers[0].HasLeft)
	require.Equal(t, created, nodes[0].CreatedAt)
}

func TestMemberTimelineRejectsInvalidInput(t *testing.T) {
	for _, input := range []MemberTimelineInput{
		{Title: "  "}, {Title: strings.Repeat("字", 61)}, {Title: "进度", Detail: strings.Repeat("字", 5001)},
		{Title: "进度", RelatedUserIDs: []int{7}}, {Title: "进度", RelatedUserIDs: []int{8, 8}}, {Title: "进度", RelatedUserIDs: []int{0}},
	} {
		require.Error(t, validateMemberTimelineInput(&input, 7))
	}
}

func TestMemberTimelineRejectsForeignRelatedMember(t *testing.T) {
	s, mock := timelineTestService(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM project_members").WithArgs(42, 7).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(100))
	mock.ExpectQuery("SELECT COALESCE").WithArgs(42, 8).WillReturnRows(sqlmock.NewRows([]string{"nickname"}))
	mock.ExpectRollback()
	_, err := s.SaveMemberTimeline(context.Background(), 42, 7, 0, MemberTimelineInput{Title: "进度", RelatedUserIDs: []int{8}})
	var business *ServiceError
	require.ErrorAs(t, err, &business)
	require.Equal(t, ErrCodeBadRequest, business.Code)
}

func TestMemberTimelineRetainsOrRemovesDepartedAssociation(t *testing.T) {
	for _, keep := range []bool{true, false} {
		t.Run(fmt.Sprint(keep), func(t *testing.T) {
			s, mock := timelineTestService(t)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT id FROM project_members").WithArgs(42, 7).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(999))
			mock.ExpectQuery("SELECT related_members FROM project_member_timeline").WithArgs(9, 42, 7).WillReturnRows(sqlmock.NewRows([]string{"related_members"}).AddRow(`[{"userId":8,"nickname":"旧伙伴"}]`))
			input := MemberTimelineInput{Title: "更新进度", Detail: "详情"}
			expected := "[]"
			if keep {
				input.RelatedUserIDs = []int{8}
				mock.ExpectQuery("SELECT COALESCE").WithArgs(42, 8).WillReturnRows(sqlmock.NewRows([]string{"nickname"}))
				expected = `[{"userId":8,"nickname":"旧伙伴","hasLeft":false}]`
			}
			mock.ExpectExec("UPDATE project_member_timeline").WithArgs("更新进度", "详情", expected, 9, 42, 7).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			id, err := s.SaveMemberTimeline(context.Background(), 42, 7, 9, input)
			require.NoError(t, err)
			require.Equal(t, 9, id)
		})
	}
}

func TestMemberTimelineCannotReAddRemovedDepartedAssociation(t *testing.T) {
	s, mock := timelineTestService(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM project_members").WithArgs(42, 7).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(999))
	mock.ExpectQuery("SELECT related_members FROM project_member_timeline").WithArgs(9, 42, 7).WillReturnRows(sqlmock.NewRows([]string{"related_members"}).AddRow(`[]`))
	mock.ExpectQuery("SELECT COALESCE").WithArgs(42, 8).WillReturnRows(sqlmock.NewRows([]string{"nickname"}))
	mock.ExpectRollback()
	_, err := s.SaveMemberTimeline(context.Background(), 42, 7, 9, MemberTimelineInput{Title: "进度", RelatedUserIDs: []int{8}})
	var business *ServiceError
	require.ErrorAs(t, err, &business)
	require.Equal(t, ErrCodeBadRequest, business.Code)
}

func TestMemberTimelineCreateUsesAuthenticatedOwner(t *testing.T) {
	s, mock := timelineTestService(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM project_members").WithArgs(42, 7).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(999))
	mock.ExpectQuery("SELECT COALESCE").WithArgs(42, 8).WillReturnRows(sqlmock.NewRows([]string{"nickname"}).AddRow("伙伴"))
	mock.ExpectExec("INSERT INTO project_member_timeline").WithArgs(42, 7, "进度", "", `[{"userId":8,"nickname":"伙伴","hasLeft":false}]`).WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectCommit()
	id, err := s.SaveMemberTimeline(context.Background(), 42, 7, 0, MemberTimelineInput{Title: "进度", RelatedUserIDs: []int{8}})
	require.NoError(t, err)
	require.Equal(t, 10, id)
}
