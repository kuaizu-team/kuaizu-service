package handler

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// OpenCustomerService redirects the stable email URL to a fresh WeChat URL Link.
func (s *Server) OpenCustomerService(c echo.Context) error {
	url, err := s.svc.CustomerServiceLink.URL(c.Request().Context())
	if err != nil {
		log.Printf("[CustomerServiceLink] generate URL Link failed: %v", err)
		return c.HTML(http.StatusServiceUnavailable,
			"<!doctype html><html lang=\"zh-CN\"><meta charset=\"utf-8\"><title>快组校园</title><body>客服入口暂时不可用，请打开“快组校园”微信小程序联系在线客服。</body></html>")
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("Referrer-Policy", "no-referrer")
	return c.Redirect(http.StatusFound, url)
}
