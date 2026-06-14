package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// RoadmapRepository handles roadmap database operations.
type RoadmapRepository struct {
	db *sqlx.DB
}

type RoadmapListParams struct {
	Page int
	Size int
}

func NewRoadmapRepository(db *sqlx.DB) *RoadmapRepository {
	return &RoadmapRepository{db: db}
}

func (r *RoadmapRepository) List(ctx context.Context) ([]models.Roadmap, error) {
	query := `
		SELECT id, date, title, content, link, created_at, updated_at
		FROM roadmap
		ORDER BY date DESC, id DESC
	`

	var items []models.Roadmap
	if err := r.db.SelectContext(ctx, &items, query); err != nil {
		return nil, fmt.Errorf("query roadmap: %w", err)
	}
	return items, nil
}

func (r *RoadmapRepository) Latest(ctx context.Context) (*models.Roadmap, error) {
	query := `
		SELECT id, date, title, content, link, created_at, updated_at
		FROM roadmap
		ORDER BY COALESCE(updated_at, created_at) DESC, id DESC
		LIMIT 1
	`
	var item models.Roadmap
	if err := r.db.QueryRowxContext(ctx, query).StructScan(&item); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query latest roadmap: %w", err)
	}
	return &item, nil
}

func (r *RoadmapRepository) HasNewForUser(ctx context.Context, userID int) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1
			FROM roadmap r
			LEFT JOIN user_roadmap_view v ON v.user_id = ?
			WHERE COALESCE(r.updated_at, r.created_at) > COALESCE(v.last_viewed_at, CAST('1970-01-01 00:00:01' AS DATETIME))
			LIMIT 1
		)
	`
	var hasNew bool
	if err := r.db.QueryRowxContext(ctx, query, userID).Scan(&hasNew); err != nil {
		return false, fmt.Errorf("query roadmap has-new: %w", err)
	}
	return hasNew, nil
}

func (r *RoadmapRepository) MarkViewed(ctx context.Context, userID int) error {
	query := `
		INSERT INTO user_roadmap_view (user_id, last_viewed_at)
		VALUES (?, CURRENT_TIMESTAMP)
		ON DUPLICATE KEY UPDATE last_viewed_at = VALUES(last_viewed_at)
	`
	if _, err := r.db.ExecContext(ctx, query, userID); err != nil {
		return fmt.Errorf("mark roadmap viewed: %w", err)
	}
	return nil
}

func (r *RoadmapRepository) AdminList(ctx context.Context, params RoadmapListParams) ([]models.Roadmap, int64, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.Size < 1 || params.Size > 100 {
		params.Size = 10
	}

	var total int64
	if err := r.db.QueryRowxContext(ctx, "SELECT COUNT(*) FROM roadmap").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count roadmap: %w", err)
	}

	query := `
		SELECT id, date, title, content, link, created_at, updated_at
		FROM roadmap
		ORDER BY date DESC, id DESC
		LIMIT ? OFFSET ?
	`
	var items []models.Roadmap
	if err := r.db.SelectContext(ctx, &items, query, params.Size, (params.Page-1)*params.Size); err != nil {
		return nil, 0, fmt.Errorf("query admin roadmap: %w", err)
	}
	return items, total, nil
}

func (r *RoadmapRepository) AdminGetByID(ctx context.Context, id int) (*models.Roadmap, error) {
	query := `
		SELECT id, date, title, content, link, created_at, updated_at
		FROM roadmap
		WHERE id = ?
	`
	var item models.Roadmap
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&item); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query roadmap by id: %w", err)
	}
	return &item, nil
}

func (r *RoadmapRepository) AdminCreate(ctx context.Context, item *models.Roadmap) error {
	query := `
		INSERT INTO roadmap (date, title, content, link)
		VALUES (:date, :title, :content, :link)
	`
	result, err := r.db.NamedExecContext(ctx, query, item)
	if err != nil {
		return fmt.Errorf("create roadmap: %w", err)
	}
	id, _ := result.LastInsertId()
	item.ID = int(id)
	return nil
}

func (r *RoadmapRepository) AdminUpdate(ctx context.Context, item *models.Roadmap) error {
	query := `
		UPDATE roadmap
		SET date = :date,
		    title = :title,
		    content = :content,
		    link = :link,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = :id
	`
	result, err := r.db.NamedExecContext(ctx, query, item)
	if err != nil {
		return fmt.Errorf("update roadmap: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *RoadmapRepository) AdminDelete(ctx context.Context, id int) error {
	if _, err := r.db.ExecContext(ctx, "DELETE FROM roadmap WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete roadmap: %w", err)
	}
	return nil
}
