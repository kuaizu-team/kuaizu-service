package handler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/labstack/echo/v4"
)

type recordingEventManagerExecer struct {
	query string
	args  []interface{}
	calls int
}

func (r *recordingEventManagerExecer) ExecContext(_ context.Context, query string, args ...interface{}) (sql.Result, error) {
	r.query = query
	r.args = append([]interface{}(nil), args...)
	r.calls++
	return eventManagerSQLResult(1), nil
}

type eventManagerSQLResult int64

func (r eventManagerSQLResult) LastInsertId() (int64, error) { return int64(r), nil }
func (r eventManagerSQLResult) RowsAffected() (int64, error) { return int64(r), nil }

func TestUpsertExistingEventManagerSyncsSchoolScope(t *testing.T) {
	schoolID := 22
	tests := []struct {
		name     string
		schoolID *int
	}{
		{name: "move to another school", schoolID: &schoolID},
		{name: "promote to non-school event", schoolID: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			managerID := 8
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/admin/events/7", nil)
			ctx := e.NewContext(req, httptest.NewRecorder())
			ctx.Set("adminRole", models.AdminRoleSuperAdmin)
			exec := &recordingEventManagerExecer{}
			event := &models.Event{Name: "Example", AdminID: &managerID, SchoolID: tt.schoolID}

			if err := (&AdminServer{}).upsertEventManager(ctx, exec, event, adminEventRequest{}); err != nil {
				t.Fatalf("upsertEventManager returned error: %v", err)
			}
			if exec.calls != 1 {
				t.Fatalf("ExecContext calls = %d, want 1", exec.calls)
			}
			if !strings.Contains(exec.query, "school_id=?") {
				t.Fatalf("update query does not synchronize school_id: %s", exec.query)
			}
			if len(exec.args) != 3 {
				t.Fatalf("update args = %d, want 3", len(exec.args))
			}
			gotSchoolID, ok := exec.args[1].(*int)
			if !ok {
				t.Fatalf("school_id argument has type %T, want *int", exec.args[1])
			}
			if tt.schoolID == nil {
				if gotSchoolID != nil {
					t.Fatalf("school_id = %d, want nil", *gotSchoolID)
				}
			} else if gotSchoolID == nil || *gotSchoolID != *tt.schoolID {
				t.Fatalf("school_id = %v, want %d", gotSchoolID, *tt.schoolID)
			}
		})
	}
}
