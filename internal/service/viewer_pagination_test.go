package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeViewerPage(t *testing.T) {
	for _, tc := range []struct {
		name                string
		page, limit         int
		wantPage, wantLimit int
	}{
		{name: "defaults", page: 0, limit: 0, wantPage: 1, wantLimit: 20},
		{name: "second page", page: 2, limit: 20, wantPage: 2, wantLimit: 20},
		{name: "limit cap", page: 3, limit: 500, wantPage: 3, wantLimit: 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, limit := normalizeViewerPage(tc.page, tc.limit)
			require.Equal(t, tc.wantPage, page)
			require.Equal(t, tc.wantLimit, limit)
		})
	}
}
