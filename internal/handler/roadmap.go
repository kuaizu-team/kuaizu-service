package handler

import (
	"log"
	"net/http"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
)

type roadmapItemVO struct {
	Date    string  `json:"date"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Link    *string `json:"link,omitempty"`
}

type roadmapListResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    []roadmapItemVO `json:"data"`
}

type roadmapHasNewResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

// ListRoadmap handles GET /roadmap.
func (s *Server) ListRoadmap(ctx echo.Context) error {
	items, err := s.repo.Roadmap.List(ctx.Request().Context())
	if err != nil {
		log.Printf("ListRoadmap error: %v", err)
		return InternalError(ctx, "get roadmap list failed")
	}

	data := make([]roadmapItemVO, 0, len(items))
	for _, item := range items {
		data = append(data, newRoadmapItemVO(item))
	}

	return ctx.JSON(http.StatusOK, roadmapListResponse{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

func (s *Server) HasNewRoadmap(ctx echo.Context) error {
	hasNew, err := s.repo.Roadmap.HasNewForUser(ctx.Request().Context(), GetUserID(ctx))
	if err != nil {
		log.Printf("HasNewRoadmap error: %v", err)
		return InternalError(ctx, "get roadmap unread status failed")
	}
	return ctx.JSON(http.StatusOK, roadmapHasNewResponse{
		Code:    0,
		Message: "success",
		Data:    map[string]interface{}{"hasNew": hasNew},
	})
}

func (s *Server) MarkRoadmapRead(ctx echo.Context) error {
	if err := s.repo.Roadmap.MarkViewed(ctx.Request().Context(), GetUserID(ctx)); err != nil {
		log.Printf("MarkRoadmapRead error: %v", err)
		return InternalError(ctx, "mark roadmap read failed")
	}
	return SuccessMessage(ctx, "success")
}

func newRoadmapItemVO(item models.Roadmap) roadmapItemVO {
	return roadmapItemVO{
		Date:    item.Date.Format("2006年1月2日"),
		Title:   item.Title,
		Content: item.Content,
		Link:    item.Link,
	}
}
