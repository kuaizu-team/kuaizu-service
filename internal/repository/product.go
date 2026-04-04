package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// ProductRepository handles product database operations
type ProductRepository struct {
	db *sqlx.DB
}

// NewProductRepository creates a new ProductRepository
func NewProductRepository(db *sqlx.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

// GetAll retrieves all products
func (r *ProductRepository) GetAll(ctx context.Context) ([]*models.Product, error) {
	query := `
		SELECT id, name, type, description, price, created_at, updated_at
		FROM product
		ORDER BY id ASC
	`

	var products []*models.Product
	if err := r.db.SelectContext(ctx, &products, query); err != nil {
		return nil, fmt.Errorf("query products: %w", err)
	}

	return products, nil
}

// GetByID retrieves a product by ID
func (r *ProductRepository) GetByID(ctx context.Context, id int) (*models.Product, error) {
	query := `
		SELECT id, name, type, description, price, created_at, updated_at
		FROM product
		WHERE id = ?
	`

	var product models.Product
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&product); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query product by id: %w", err)
	}

	return &product, nil
}
