package handler

import (
	"log"
	"net/http"

	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
)

type informationListResponse struct {
	Code    int                        `json:"code"`
	Message string                     `json:"message"`
	Data    []api.InformationContentVO `json:"data"`
}

// ListInformation handles GET /information/list.
func (s *Server) ListInformation(ctx echo.Context, params api.ListInformationParams) error {
	category := string(params.Category)
	if !models.IsValidInformationCategory(category) {
		return BadRequest(ctx, "invalid category")
	}

	items, err := s.repo.InformationContent.ListPublishedByCategory(ctx.Request().Context(), category, 4)
	if err != nil {
		log.Printf("ListInformation error: %v", err)
		return InternalError(ctx, "get information list failed")
	}

	data := make([]api.InformationContentVO, 0, len(items))
	for _, item := range items {
		data = append(data, item.ToVO())
	}

	return ctx.JSON(http.StatusOK, informationListResponse{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}
