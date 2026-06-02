package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// InformationContentRepository handles information-center database operations.
type InformationContentRepository struct {
	db *sqlx.DB
}

// NewInformationContentRepository creates a new InformationContentRepository.
func NewInformationContentRepository(db *sqlx.DB) *InformationContentRepository {
	return &InformationContentRepository{db: db}
}

// ListPublishedByCategory returns the latest published information items for a category.
func (r *InformationContentRepository) ListPublishedByCategory(ctx context.Context, category string, limit int) ([]models.InformationContent, error) {
	query := `
		SELECT id, COALESCE(title, '') AS title, COALESCE(url, '') AS url, COALESCE(content, '') AS content, created_at
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
