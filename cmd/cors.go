package cmd

import (
	"net/http"
	"os"
	"strings"

	echomiddleware "github.com/labstack/echo/v4/middleware"
)

func CORSConfig(envKey string, defaults []string) echomiddleware.CORSConfig {
	origins := append([]string(nil), defaults...)
	if configured := strings.TrimSpace(os.Getenv(envKey)); configured != "" {
		origins = make([]string, 0, len(defaults))
		for _, origin := range strings.Split(configured, ",") {
			if origin = strings.TrimSpace(origin); origin != "" {
				origins = append(origins, origin)
			}
		}
	}
	return echomiddleware.CORSConfig{
		AllowOrigins: origins,
		AllowMethods: []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{"Accept", "Authorization", "Content-Type", "Origin"},
	}
}
