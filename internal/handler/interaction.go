package handler

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/labstack/echo/v4"
)

type favoriteProjectVO struct {
	*api.ProjectVO
	FavoritedAt time.Time `json:"favoritedAt"`
}

type favoriteTalentVO struct {
	*api.TalentProfileVO
	FavoritedAt time.Time `json:"favoritedAt"`
}

func interactionParams(ctx echo.Context) (int, int, int) {
	page, _ := strconv.Atoi(ctx.QueryParam("page"))
	size, _ := strconv.Atoi(ctx.QueryParam("size"))
	days, _ := strconv.Atoi(ctx.QueryParam("days"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if days < 1 {
		days = 30
	}
	return page, size, days
}

func (s *Server) GetInteractions(ctx echo.Context, target string) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return BadRequest(ctx, "invalid target id")
	}
	result, svcErr := s.svc.Interaction.Get(ctx.Request().Context(), target, id, GetUserID(ctx))
	if svcErr != nil {
		return mapServiceError(ctx, svcErr)
	}
	return Success(ctx, result)
}

func (s *Server) ToggleInteraction(ctx echo.Context, target, kind string) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return BadRequest(ctx, "invalid target id")
	}
	result, svcErr := s.svc.Interaction.Toggle(ctx.Request().Context(), target, kind, id, GetUserID(ctx))
	if svcErr != nil {
		return mapServiceError(ctx, svcErr)
	}
	return Success(ctx, result)
}

func (s *Server) ShareInteraction(ctx echo.Context, target string) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return BadRequest(ctx, "invalid target id")
	}
	var req struct {
		Channel string `json:"channel"`
	}
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "invalid request")
	}
	result, svcErr := s.svc.Interaction.Share(ctx.Request().Context(), target, id, GetUserID(ctx), req.Channel)
	if svcErr != nil {
		return mapServiceError(ctx, svcErr)
	}
	return Success(ctx, result)
}

func (s *Server) ListInteractionUsers(ctx echo.Context, target string) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return BadRequest(ctx, "invalid target id")
	}
	page, size, days := interactionParams(ctx)
	result, svcErr := s.svc.Interaction.ListUsers(ctx.Request().Context(), target, ctx.QueryParam("type"), id, page, size, days)
	if svcErr != nil {
		return mapServiceError(ctx, svcErr)
	}
	return Success(ctx, result)
}

func (s *Server) GetFavoriteUnreadCount(ctx echo.Context) error {
	state, err := s.repo.Interaction.UnreadFavorites(ctx.Request().Context(), GetUserID(ctx))
	if err != nil {
		return InternalError(ctx, "get unread favorite count failed")
	}
	return Success(ctx, state)
}

func (s *Server) MarkFavoritesViewed(ctx echo.Context) error {
	var req struct {
		Target string `json:"target"`
	}
	if err := ctx.Bind(&req); err != nil {
		return BadRequest(ctx, "invalid request")
	}
	if req.Target != repository.InteractionProject && req.Target != repository.InteractionTalent {
		return BadRequest(ctx, "invalid target")
	}
	if err := s.repo.Interaction.MarkFavoritesViewed(ctx.Request().Context(), GetUserID(ctx), req.Target); err != nil {
		return InternalError(ctx, "mark favorites viewed failed")
	}
	return Success(ctx, nil)
}

func (s *Server) GetDashboardUnreadCount(ctx echo.Context) error {
	totals, err := s.svc.Interaction.DashboardUnreadTotals(ctx.Request().Context(), GetUserID(ctx))
	if err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, totals)
}

func (s *Server) MarkDashboardViewed(ctx echo.Context, target string) error {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return BadRequest(ctx, "invalid target id")
	}
	var req struct {
		Type *string `json:"type"`
	}
	if err := json.NewDecoder(ctx.Request().Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		return BadRequest(ctx, "invalid request")
	}
	if err := s.svc.Interaction.MarkDashboardViewed(ctx.Request().Context(), target, id, GetUserID(ctx), req.Type); err != nil {
		return mapServiceError(ctx, err)
	}
	return Success(ctx, nil)
}

func (s *Server) ListFavorites(ctx echo.Context, target string) error {
	page, size, _ := interactionParams(ctx)
	userID := GetUserID(ctx)
	if target == repository.InteractionProject {
		items, total, err := s.repo.Interaction.ListFavoriteProjects(ctx.Request().Context(), userID, page, size)
		if err != nil {
			return InternalError(ctx, "list favorite projects failed")
		}
		ids := make([]int, len(items))
		for i := range items {
			ids[i] = items[i].ID
		}
		stats, err := s.repo.Interaction.Batch(ctx.Request().Context(), target, ids, userID)
		if err != nil {
			return InternalError(ctx, "get interactions failed")
		}
		list := make([]favoriteProjectVO, len(items))
		for i := range items {
			items[i].Interaction = stats[items[i].ID]
			list[i] = favoriteProjectVO{items[i].ToVO(), items[i].FavoritedAt}
		}
		return Success(ctx, map[string]interface{}{"list": list, "page": page, "size": size, "total": total})
	}
	items, total, err := s.repo.Interaction.ListFavoriteTalents(ctx.Request().Context(), userID, page, size)
	if err != nil {
		return InternalError(ctx, "list favorite talent profiles failed")
	}
	ids := make([]int, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	stats, err := s.repo.Interaction.Batch(ctx.Request().Context(), target, ids, userID)
	if err != nil {
		return InternalError(ctx, "get interactions failed")
	}
	list := make([]favoriteTalentVO, len(items))
	for i := range items {
		items[i].Interaction = stats[items[i].ID]
		list[i] = favoriteTalentVO{items[i].ToVO(), items[i].FavoritedAt}
	}
	return Success(ctx, map[string]interface{}{"list": list, "page": page, "size": size, "total": total})
}

func (s *Server) GetInteractionsFavoritesUnreadCount(ctx echo.Context) error {
	return s.GetFavoriteUnreadCount(ctx)
}
func (s *Server) GetInteractionsDashboardUnreadCount(ctx echo.Context) error {
	return s.GetDashboardUnreadCount(ctx)
}
func (s *Server) PostInteractionsFavoritesMarkViewed(ctx echo.Context) error {
	return s.MarkFavoritesViewed(ctx)
}
func (s *Server) GetProjectsFavorites(ctx echo.Context, _ api.GetProjectsFavoritesParams) error {
	return s.ListFavorites(ctx, repository.InteractionProject)
}
func (s *Server) GetTalentProfilesFavorites(ctx echo.Context, _ api.GetTalentProfilesFavoritesParams) error {
	return s.ListFavorites(ctx, repository.InteractionTalent)
}
func (s *Server) GetProjectsIdInteractions(ctx echo.Context, _ int) error {
	return s.GetInteractions(ctx, repository.InteractionProject)
}
func (s *Server) GetTalentProfilesIdInteractions(ctx echo.Context, _ int) error {
	return s.GetInteractions(ctx, repository.InteractionTalent)
}
func (s *Server) PostProjectsIdLike(ctx echo.Context, _ int) error {
	return s.ToggleInteraction(ctx, repository.InteractionProject, "like")
}
func (s *Server) PostTalentProfilesIdLike(ctx echo.Context, _ int) error {
	return s.ToggleInteraction(ctx, repository.InteractionTalent, "like")
}
func (s *Server) PostProjectsIdFavorite(ctx echo.Context, _ int) error {
	return s.ToggleInteraction(ctx, repository.InteractionProject, "favorite")
}
func (s *Server) PostTalentProfilesIdFavorite(ctx echo.Context, _ int) error {
	return s.ToggleInteraction(ctx, repository.InteractionTalent, "favorite")
}
func (s *Server) PostProjectsIdShare(ctx echo.Context, _ int) error {
	return s.ShareInteraction(ctx, repository.InteractionProject)
}
func (s *Server) PostTalentProfilesIdShare(ctx echo.Context, _ int) error {
	return s.ShareInteraction(ctx, repository.InteractionTalent)
}
func (s *Server) GetProjectsIdInteractionUsers(ctx echo.Context, _ int, _ api.GetProjectsIdInteractionUsersParams) error {
	return s.ListInteractionUsers(ctx, repository.InteractionProject)
}
func (s *Server) GetTalentProfilesIdInteractionUsers(ctx echo.Context, _ int, _ api.GetTalentProfilesIdInteractionUsersParams) error {
	return s.ListInteractionUsers(ctx, repository.InteractionTalent)
}
func (s *Server) PostProjectsIdInteractionsMarkViewed(ctx echo.Context, _ int) error {
	return s.MarkDashboardViewed(ctx, repository.InteractionProject)
}
func (s *Server) PostTalentProfilesIdInteractionsMarkViewed(ctx echo.Context, _ int) error {
	return s.MarkDashboardViewed(ctx, repository.InteractionTalent)
}
