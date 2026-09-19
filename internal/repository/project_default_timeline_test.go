package repository

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"strings"
	"testing"
)

func TestDefaultTimelineVisibilityWritesOnlyExplicitProjectFlag(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		db := openCaptureDB(t)
		repo := NewProjectRepository(sqlx.NewDb(db, "capture_user_repo"))
		p := &models.Project{ID: 42, Name: "project", DefaultTimelineHidden: true}
		hidden := true
		if explicit {
			p.DefaultTimelineHiddenUpdate = &hidden
		}
		_, err := repo.UpdateWithMetadata(context.Background(), p, nil, nil, nil, nil, nil, nil, 7, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		capturedExec.Lock()
		query := normalizeSQL(capturedExec.query)
		if explicit {
			if query != "UPDATE project SET default_timeline_hidden=? WHERE id=?" {
				t.Errorf("unexpected visibility write: %s", query)
			}
			if len(capturedExec.args) != 2 || capturedExec.args[0].Value != true || capturedExec.args[1].Value != int64(42) {
				t.Errorf("unexpected visibility args: %#v", capturedExec.args)
			}
		} else if strings.Contains(query, "default_timeline_hidden") {
			t.Error("omitted field overwrites persisted visibility")
		}
		if strings.Contains(query, "created_at") || strings.Contains(query, "project_milestones") {
			t.Error("visibility update changes creation date or milestones")
		}
		capturedExec.Unlock()
		db.Close()
	}
}
