package handler

import (
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
)

type operationsSummaryResponse struct {
	OperatedSchoolCount int `json:"operatedSchoolCount"`
}

type operationsTeamResponse struct {
	OperatedSchoolCount   int                               `json:"operatedSchoolCount"`
	CurrentUserSchoolID   *int                              `json:"currentUserSchoolId,omitempty"`
	CurrentUserSchoolName *string                           `json:"currentUserSchoolName,omitempty"`
	CurrentSchoolLeader   *repository.OperationsPerson      `json:"currentSchoolLeader,omitempty"`
	Teams                 []repository.OperationsSchoolTeam `json:"teams"`
}

func (s *Server) GetOperationsSummary(ctx echo.Context) error {
	count, err := s.repo.Operations.CountOperatedSchools(ctx.Request().Context())
	if err != nil {
		ctx.Logger().Errorf("get operations summary: %v", err)
		return InternalError(ctx, "获取运营范围失败")
	}
	return Success(ctx, operationsSummaryResponse{OperatedSchoolCount: count})
}

func (s *Server) ListOperationsTeam(ctx echo.Context) error {
	teams, err := s.repo.Operations.ListSchoolTeams(ctx.Request().Context())
	if err != nil {
		ctx.Logger().Errorf("list operations team: %v", err)
		return InternalError(ctx, "获取运营团队失败")
	}
	response := operationsTeamResponse{
		OperatedSchoolCount: len(teams),
		Teams:               teams,
	}

	user, userErr := s.repo.User.GetByID(ctx.Request().Context(), GetUserID(ctx))
	if userErr != nil {
		ctx.Logger().Errorf("get operations viewer: %v", userErr)
		return InternalError(ctx, "获取当前用户学校失败")
	}
	if user != nil && user.SchoolID != nil {
		response.CurrentUserSchoolID = user.SchoolID
		response.CurrentUserSchoolName = user.SchoolName
		for i := range teams {
			if teams[i].SchoolID == *user.SchoolID {
				leader := teams[i].Leader
				response.CurrentSchoolLeader = &leader
				break
			}
		}
	}

	return Success(ctx, response)
}
