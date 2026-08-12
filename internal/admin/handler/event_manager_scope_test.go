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

func TestCreateEventManagerRequiresPhone(t *testing.T) {
	account := "event_manager"
	password := "secret123"
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/events", nil)
	rec := httptest.NewRecorder()
	ctx := e.NewContext(req, rec)
	ctx.Set("adminRole", models.AdminRoleSuperAdmin)
	exec := &recordingEventManagerExecer{}
	event := &models.Event{Name: "Example"}

	err := (&AdminServer{}).upsertEventManager(ctx, exec, event, adminEventRequest{
		ManagerAccount:  &account,
		ManagerPassword: &password,
	})
	if err != nil {
		t.Fatalf("upsertEventManager returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if exec.calls != 0 {
		t.Fatalf("ExecContext calls = %d, want 0", exec.calls)
	}
}

func TestSchoolAdminEventRequestForcesCurrentSchool(t *testing.T) {
	schoolID := 22
	requestSchoolID := 22
	level := "school"
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/events", nil)
	ctx := e.NewContext(req, httptest.NewRecorder())
	ctx.Set("adminRole", models.AdminRoleSchoolAdmin)
	ctx.Set("adminSchoolID", schoolID)

	event, err := (&AdminServer{}).buildAdminEventModelForRequest(ctx, adminEventRequest{
		Name: "School Event", Level: &level, SchoolID: &requestSchoolID,
	})
	if err != nil {
		t.Fatalf("buildAdminEventModelForRequest returned error: %v", err)
	}
	if event.Level == nil || *event.Level != "school" ||
		event.SchoolID == nil || *event.SchoolID != schoolID {
		t.Fatalf("event scope = (%v, %v), want school/%d", event.Level, event.SchoolID, schoolID)
	}

	otherSchoolID := 23
	if _, err := (&AdminServer{}).buildAdminEventModelForRequest(ctx, adminEventRequest{
		Name: "Other School Event", Level: &level, SchoolID: &otherSchoolID,
	}); err == nil {
		t.Fatal("school admin could submit another schoolId")
	}

	national := "national"
	if _, err := (&AdminServer{}).buildAdminEventModelForRequest(ctx, adminEventRequest{
		Name: "National Event", Level: &national,
	}); err == nil {
		t.Fatal("school admin could create a national event")
	}
}

func TestSchoolAdminCannotUpdateExistingEventManager(t *testing.T) {
	schoolID := 22
	managerID := 8
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/admin/events/7", nil)
	ctx := e.NewContext(req, httptest.NewRecorder())
	ctx.Set("adminRole", models.AdminRoleSchoolAdmin)
	ctx.Set("adminSchoolID", schoolID)
	exec := &recordingEventManagerExecer{}
	level := "school"
	event := &models.Event{Name: "School Event", Level: &level, AdminID: &managerID, SchoolID: &schoolID}

	rec := httptest.NewRecorder()
	ctx = e.NewContext(req, rec)
	ctx.Set("adminRole", models.AdminRoleSchoolAdmin)
	ctx.Set("adminSchoolID", schoolID)
	if err := (&AdminServer{}).upsertEventManager(ctx, exec, event, adminEventRequest{}); err != nil {
		t.Fatalf("upsertEventManager returned error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if exec.calls != 0 {
		t.Fatalf("ExecContext calls = %d, want 0", exec.calls)
	}
}

func TestSchoolAdminCanCreateManagerForOwnSchoolEvent(t *testing.T) {
	schoolID := 22
	level := "school"
	account, password, phone := "school_event_manager", "secret123", "13800138000"
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/events", nil)
	ctx := e.NewContext(req, httptest.NewRecorder())
	ctx.Set("adminRole", models.AdminRoleSchoolAdmin)
	ctx.Set("adminSchoolID", schoolID)
	exec := &recordingEventManagerExecer{}
	event := &models.Event{ID: 7, Name: "School Event", Level: &level, SchoolID: &schoolID}

	if err := (&AdminServer{}).upsertEventManager(ctx, exec, event, adminEventRequest{
		ManagerAccount: &account, ManagerPassword: &password, ManagerPhone: &phone,
	}); err != nil {
		t.Fatalf("upsertEventManager returned error: %v", err)
	}
	if exec.calls != 2 {
		t.Fatalf("ExecContext calls = %d, want 2", exec.calls)
	}
}
