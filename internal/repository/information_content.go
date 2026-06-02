package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// InformationContentRepository handles information-center database operations.
type InformationContentRepository struct {
	db *sqlx.DB
}

// InformationContentListParams contains parameters for admin listing.
type InformationContentListParams struct {
	Page     int
	Size     int
	Category *string
}

// NewInformationContentRepository creates a new InformationContentRepository.
func NewInformationContentRepository(db *sqlx.DB) *InformationContentRepository {
	return &InformationContentRepository{db: db}
}

// ListPublishedByCategory returns the latest published information items for a category.
func (r *InformationContentRepository) ListPublishedByCategory(ctx context.Context, category string, limit int) ([]models.InformationContent, error) {
	query := `
		SELECT id, COALESCE(title, '') AS title, COALESCE(url, '') AS url, COALESCE(content, '') AS content,
		       category, display_order, is_published, created_at, updated_at
		FROM information_content
		WHERE category = ? AND is_published = 1
		ORDER BY display_order DESC, created_at DESC, id DESC
		LIMIT ?
	`

	var items []models.InformationContent
	if err := r.db.SelectContext(ctx, &items, query, category, limit); err != nil {
		return nil, fmt.Errorf("query information content: %w", err)
	}

	return items, nil
}

// AdminList retrieves all information items for admin management.
func (r *InformationContentRepository) AdminList(ctx context.Context, params InformationContentListParams) ([]models.InformationContent, int64, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.Size < 1 || params.Size > 100 {
		params.Size = 10
	}

	conditions := []string{"1=1"}
	args := []interface{}{}
	if params.Category != nil && *params.Category != "" {
		conditions = append(conditions, "category = ?")
		args = append(args, *params.Category)
	}
	whereClause := strings.Join(conditions, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM information_content WHERE %s", whereClause)
	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count information content: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, COALESCE(title, '') AS title, COALESCE(url, '') AS url, COALESCE(content, '') AS content,
		       category, display_order, is_published, created_at, updated_at
		FROM information_content
		WHERE %s
		ORDER BY display_order DESC, created_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, whereClause)
	args = append(args, params.Size, (params.Page-1)*params.Size)

	var items []models.InformationContent
	if err := r.db.SelectContext(ctx, &items, query, args...); err != nil {
		return nil, 0, fmt.Errorf("query admin information content: %w", err)
	}
	return items, total, nil
}

// AdminGetByID retrieves a single information item for admin management.
func (r *InformationContentRepository) AdminGetByID(ctx context.Context, id int) (*models.InformationContent, error) {
	query := `
		SELECT id, COALESCE(title, '') AS title, COALESCE(url, '') AS url, COALESCE(content, '') AS content,
		       category, display_order, is_published, created_at, updated_at
		FROM information_content
		WHERE id = ?
	`
	var item models.InformationContent
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&item); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query information content by id: %w", err)
	}
	return &item, nil
}

// AdminCreate inserts a new information item.
func (r *InformationContentRepository) AdminCreate(ctx context.Context, item *models.InformationContent) error {
	query := `
		INSERT INTO information_content (title, url, content, category, display_order, is_published)
		VALUES (:title, :url, :content, :category, :display_order, :is_published)
	`
	result, err := r.db.NamedExecContext(ctx, query, item)
	if err != nil {
		return fmt.Errorf("create information content: %w", err)
	}
	id, _ := result.LastInsertId()
	item.ID = int(id)
	return nil
}

// AdminUpdate replaces editable fields for an information item.
func (r *InformationContentRepository) AdminUpdate(ctx context.Context, item *models.InformationContent) error {
	query := `
		UPDATE information_content
		SET title = :title,
		    url = :url,
		    content = :content,
		    category = :category,
		    display_order = :display_order,
		    is_published = :is_published,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = :id
	`
	result, err := r.db.NamedExecContext(ctx, query, item)
	if err != nil {
		return fmt.Errorf("update information content: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// AdminDelete deletes an information item. Missing rows are treated as a no-op.
func (r *InformationContentRepository) AdminDelete(ctx context.Context, id int) error {
	if _, err := r.db.ExecContext(ctx, "DELETE FROM information_content WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete information content: %w", err)
	}
	return nil
}
