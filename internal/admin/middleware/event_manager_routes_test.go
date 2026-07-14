package middleware

import (
	"net/http"
	"testing"
)

func TestEventManagerRouteAllowed(t *testing.T) {
	allowed := []struct{ method, path string }{
		{http.MethodGet, "/admin/auth/me"},
		{http.MethodGet, "/admin/dashboard/stats"},
		{http.MethodGet, "/admin/projects/12"},
		{http.MethodGet, "/admin/projects/12/applications"},
		{http.MethodGet, "/admin/users/88"},
		{http.MethodPatch, "/admin/projects/12"},
		{http.MethodPatch, "/admin/projects/12/restore"},
		{http.MethodDelete, "/admin/projects/12/permanent"},
	}
	for _, tc := range allowed {
		if !eventManagerRouteAllowed(tc.method, tc.path) {
			t.Errorf("expected %s %s to be allowed", tc.method, tc.path)
		}
	}

	denied := []struct{ method, path string }{
		{http.MethodGet, "/admin/users"},
		{http.MethodPut, "/admin/users/88/status"},
		{http.MethodPatch, "/admin/projects/12/takedown"},
		{http.MethodGet, "/admin/admins/1"},
	}
	for _, tc := range denied {
		if eventManagerRouteAllowed(tc.method, tc.path) {
			t.Errorf("expected %s %s to be denied", tc.method, tc.path)
		}
	}
}
