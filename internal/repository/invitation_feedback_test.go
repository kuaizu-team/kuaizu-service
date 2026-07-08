package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

func TestInvitationFeedbackUpsertConversationStatusClearsFeedbackWhenInProgress(t *testing.T) {
	db := openCaptureDB(t)
	defer db.Close()
	repo := NewInvitationFeedbackRepository(sqlx.NewDb(db, "capture_user_repo"))

	capturedExec.Lock()
	capturedExec.query = ""
	capturedExec.args = nil
	capturedExec.Unlock()
	setCapturedQuery(
		[]string{"id", "user_id", "status", "intention_text", "conversation_status", "created_at", "updated_at"},
		[][]driver.Value{{int64(1), int64(1001), models.InvitationFeedbackStatusPending, nil, models.InvitationConversationStatusInProgress, nil, nil}},
	)

	if _, err := repo.UpsertConversationStatus(context.Background(), 1001, models.InvitationConversationStatusInProgress); err != nil {
		t.Fatalf("UpsertConversationStatus returned error: %v", err)
	}

	capturedExec.Lock()
	query := normalizeSQL(capturedExec.query)
	args := append([]driver.NamedValue(nil), capturedExec.args...)
	capturedExec.Unlock()

	for _, want := range []string{
		"status = IF(VALUES(conversation_status) = 'in_progress', VALUES(status), status)",
		"intention_text = IF(VALUES(conversation_status) = 'in_progress', NULL, intention_text)",
		"conversation_status = VALUES(conversation_status)",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("query = %q, missing %q", query, want)
		}
	}
	if len(args) != 3 || args[0].Value != int64(1001) || args[1].Value != models.InvitationFeedbackStatusPending || args[2].Value != models.InvitationConversationStatusInProgress {
		t.Fatalf("args = %#v", args)
	}
}
