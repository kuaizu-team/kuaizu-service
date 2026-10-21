package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestAdminEndProjectArchivesAtomically(t *testing.T) {
	for _, tc := range []struct {
		name       string
		rows       int64
		updateErr  error
		archiveErr error
		commitErr  error
		want       int
	}{
		{name: "state and creator message commit together", rows: 1, want: http.StatusOK},
		{name: "missing or ineligible project cannot archive", want: http.StatusBadRequest},
		{name: "update failure rolls back", updateErr: errors.New("update failed"), want: http.StatusInternalServerError},
		{name: "archive failure rolls back state", rows: 1, archiveErr: errors.New("archive failed"), want: http.StatusInternalServerError},
		{name: "commit failure cannot report success", rows: 1, commitErr: errors.New("commit failed"), want: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer raw.Close()
			server := &AdminServer{repo: repository.New(sqlx.NewDb(raw, "mysql"))}
			mock.ExpectBegin()
			update := mock.ExpectExec(regexp.QuoteMeta("UPDATE project SET status=?,ended_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status IN (?,?)")).
				WithArgs(models.ProjectStatusEnded, 7, models.ProjectStatusApproved, models.ProjectStatusRecruitCompleted)
			if tc.updateErr != nil {
				update.WillReturnError(tc.updateErr)
			} else {
				update.WillReturnResult(sqlmock.NewResult(0, tc.rows))
			}
			if tc.updateErr == nil && tc.rows > 0 {
				// The recipient must be the project creator, never the administrator ID.
				archive := mock.ExpectExec("(?s)INSERT INTO user_status_message.*SELECT p.creator_id,\\?,CONCAT\\('project-ended:',p.id\\).*u.id=p.creator_id WHERE p.id=\\?").
					WithArgs("project-ended", 7)
				if tc.archiveErr != nil {
					archive.WillReturnError(tc.archiveErr)
				} else {
					archive.WillReturnResult(sqlmock.NewResult(9, 1))
				}
			}
			if tc.updateErr != nil || tc.rows == 0 || tc.archiveErr != nil {
				mock.ExpectRollback()
			} else if tc.commitErr != nil {
				mock.ExpectCommit().WillReturnError(tc.commitErr)
			} else {
				mock.ExpectCommit()
			}
			rec := httptest.NewRecorder()
			ctx := echo.New().NewContext(httptest.NewRequest(http.MethodPatch, "/admin/projects/7/status", strings.NewReader(`{"status":5}`)), rec)
			ctx.Request().Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			ctx.SetParamNames("id")
			ctx.SetParamValues("7")
			ctx.Set("adminRole", models.AdminRoleSuperAdmin)
			ctx.Set("adminID", 99)
			require.NoError(t, server.UpdateProjectLifecycleStatus(ctx))
			require.Equal(t, tc.want, rec.Code)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAdminEndProjectRemainsSuperAdminOnly(t *testing.T) {
	for _, role := range []int{0, models.AdminRoleSchoolSuperAdmin, models.AdminRoleSchoolAdmin, models.AdminRoleEventManager} {
		rec := httptest.NewRecorder()
		ctx := echo.New().NewContext(httptest.NewRequest(http.MethodPatch, "/admin/projects/7/status", strings.NewReader(`{"status":5}`)), rec)
		ctx.Set("adminRole", role)
		require.NoError(t, (&AdminServer{}).UpdateProjectLifecycleStatus(ctx))
		require.Equal(t, http.StatusForbidden, rec.Code)
	}
}
