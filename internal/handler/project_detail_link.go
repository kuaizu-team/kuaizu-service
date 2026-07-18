package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/kuaizu-team/kuaizu-service/internal/service"
	"github.com/labstack/echo/v4"
)

const projectDetailLinkFallbackHTML = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>快组校园</title></head><body style="margin:0;padding:40px 24px;font-family:-apple-system,BlinkMacSystemFont,'PingFang SC','Helvetica Neue',Arial,sans-serif;color:#1d1d1f;"><main style="max-width:560px;margin:0 auto;"><h1 style="font-size:26px;">暂时无法打开项目</h1><p style="font-size:16px;line-height:1.8;color:#424245;">请打开“快组校园”微信小程序，搜索项目名称后进入详情页并投递名片。</p></main></body></html>`

// OpenProjectDetail redirects the stable email URL to a project-specific WeChat URL Link.
func (s *Server) OpenProjectDetail(c echo.Context) error {
	projectID, err := strconv.Atoi(c.QueryParam("id"))
	if err != nil || projectID <= 0 {
		return c.HTML(http.StatusBadRequest, projectDetailLinkFallbackHTML)
	}

	source := 2
	if rawSource := c.QueryParam("source"); rawSource != "" {
		source, err = strconv.Atoi(rawSource)
		if err != nil || source != 2 {
			return c.HTML(http.StatusBadRequest, projectDetailLinkFallbackHTML)
		}
	}

	targetURL, err := s.svc.ProjectDetailLink.URL(c.Request().Context(), projectID, source)
	if err != nil {
		if errors.Is(err, service.ErrProjectDetailLinkNotFound) {
			return c.HTML(http.StatusNotFound, projectDetailLinkFallbackHTML)
		}
		log.Printf("[ProjectDetailLink] generate URL Link failed project_id=%d source=%d: %v", projectID, source, err)
		return c.HTML(http.StatusServiceUnavailable, projectDetailLinkFallbackHTML)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("Referrer-Policy", "no-referrer")
	return c.Redirect(http.StatusFound, targetURL)
}
