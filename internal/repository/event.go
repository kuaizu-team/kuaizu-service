package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

type EventRepository struct {
	db *sqlx.DB
}

type EventListParams struct {
	Page    int
	Size    int
	Keyword *string
}

func NewEventRepository(db *sqlx.DB) *EventRepository {
	return &EventRepository{db: db}
}

func normalizeEventPage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}
	return page, size
}

func eventSelectColumns() string {
	return `id, name, is_ranking, registration_deadline, article_url, display_order, created_at, updated_at`
}

func (r *EventRepository) List(ctx context.Context, params EventListParams) ([]models.Event, int64, error) {
	params.Page, params.Size = normalizeEventPage(params.Page, params.Size)
	conditions := []string{"1=1"}
	args := []interface{}{}
	if params.Keyword != nil && strings.TrimSpace(*params.Keyword) != "" {
		conditions = append(conditions, "name LIKE ?")
		args = append(args, "%"+strings.TrimSpace(*params.Keyword)+"%")
	}
	where := strings.Join(conditions, " AND ")

	var total int64
	if err := r.db.QueryRowxContext(ctx, "SELECT COUNT(*) FROM event WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count events: %w", err)
	}

	query := fmt.Sprintf(`SELECT %s FROM event WHERE %s ORDER BY display_order DESC, created_at DESC, id DESC LIMIT ? OFFSET ?`, eventSelectColumns(), where)
	args = append(args, params.Size, (params.Page-1)*params.Size)
	var events []models.Event
	if err := r.db.SelectContext(ctx, &events, query, args...); err != nil {
		return nil, 0, fmt.Errorf("query events: %w", err)
	}
	return events, total, nil
}

func (r *EventRepository) ListTimeline(ctx context.Context, limit int) ([]models.Event, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query := fmt.Sprintf(`SELECT %s FROM event ORDER BY display_order ASC, created_at ASC, id ASC LIMIT ?`, eventSelectColumns())
	var events []models.Event
	if err := r.db.SelectContext(ctx, &events, query, limit); err != nil {
		return nil, fmt.Errorf("query event timeline: %w", err)
	}
	return events, nil
}

func (r *EventRepository) GetByID(ctx context.Context, id int) (*models.Event, error) {
	query := fmt.Sprintf(`SELECT %s FROM event WHERE id = ?`, eventSelectColumns())
	var event models.Event
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&event); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query event by id: %w", err)
	}
	return &event, nil
}

func (r *EventRepository) Create(ctx context.Context, event *models.Event) error {
	query := `
		INSERT INTO event (name, is_ranking, registration_deadline, article_url, display_order)
		VALUES (:name, :is_ranking, :registration_deadline, :article_url, :display_order)
	`
	if !event.CreatedAt.IsZero() {
		query = `
			INSERT INTO event (name, is_ranking, registration_deadline, article_url, display_order, created_at)
			VALUES (:name, :is_ranking, :registration_deadline, :article_url, :display_order, :created_at)
		`
	}
	result, err := r.db.NamedExecContext(ctx, query, event)
	if err != nil {
		return fmt.Errorf("create event: %w", err)
	}
	id, _ := result.LastInsertId()
	event.ID = int(id)
	return nil
}

func (r *EventRepository) Update(ctx context.Context, event *models.Event) error {
	query := `
		UPDATE event
		SET name = :name,
		    is_ranking = :is_ranking,
		    registration_deadline = :registration_deadline,
		    article_url = :article_url,
		    display_order = :display_order,
		    created_at = :created_at,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = :id
	`
	result, err := r.db.NamedExecContext(ctx, query, event)
	if err != nil {
		return fmt.Errorf("update event: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *EventRepository) Delete(ctx context.Context, id int) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM project_event WHERE event_id = ?", id); err != nil {
		return fmt.Errorf("delete project event relations: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM information_event WHERE event_id = ?", id); err != nil {
		return fmt.Errorf("delete information event relations: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM event WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete event: %w", err)
	}
	return tx.Commit()
}

func (r *EventRepository) ListByProjectIDs(ctx context.Context, projectIDs []int) (map[int][]models.Event, error) {
	result := map[int][]models.Event{}
	if len(projectIDs) == 0 {
		return result, nil
	}
	query, args, err := sqlx.In(`SELECT pe.project_id, e.id, e.name, e.is_ranking, e.registration_deadline, e.article_url, e.display_order, e.created_at, e.updated_at
		FROM project_event pe
		JOIN event e ON e.id = pe.event_id
		WHERE pe.project_id IN (?)
		ORDER BY e.display_order DESC, e.created_at DESC, e.id DESC`, projectIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("query project events: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var projectID int
		var event models.Event
		if err := rows.Scan(&projectID, &event.ID, &event.Name, &event.IsRanking, &event.RegistrationDeadline, &event.ArticleURL, &event.DisplayOrder, &event.CreatedAt, &event.UpdatedAt); err != nil {
			return nil, err
		}
		result[projectID] = append(result[projectID], event)
	}
	return result, rows.Err()
}

func (r *EventRepository) ListProjectIDs(ctx context.Context, eventID int) ([]int, error) {
	var ids []int
	if err := r.db.SelectContext(ctx, &ids, `SELECT project_id FROM project_event WHERE event_id = ? ORDER BY created_at DESC`, eventID); err != nil {
		return nil, fmt.Errorf("query event project ids: %w", err)
	}
	return ids, nil
}

func (r *EventRepository) ReplaceProjectEventsTx(ctx context.Context, tx *sqlx.Tx, projectID int, eventIDs []int) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM project_event WHERE project_id = ?", projectID); err != nil {
		return err
	}
	for _, eventID := range uniquePositiveIDs(eventIDs) {
		if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO project_event(project_id, event_id) VALUES(?, ?)`, projectID, eventID); err != nil {
			return err
		}
	}
	return nil
}

func uniquePositiveIDs(ids []int) []int {
	seen := map[int]struct{}{}
	result := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
