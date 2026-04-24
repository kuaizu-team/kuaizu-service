package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// AdminUserRepository handles admin user database operations
type AdminUserRepository struct {
	db *sqlx.DB
}

// NewAdminUserRepository creates a new AdminUserRepository
func NewAdminUserRepository(db *sqlx.DB) *AdminUserRepository {
	return &AdminUserRepository{db: db}
}

// GetByUsername retrieves an admin user by username
func (r *AdminUserRepository) GetByUsername(ctx context.Context, username string) (*models.AdminUser, error) {
	query := `
		SELECT id, username, password_hash, nickname, status, created_at, updated_at
		FROM admin_user
		WHERE username = ?
	`

	var admin models.AdminUser
	if err := r.db.QueryRowxContext(ctx, query, username).StructScan(&admin); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query admin by username: %w", err)
	}

	return &admin, nil
}

// GetByID retrieves an admin user by ID
func (r *AdminUserRepository) GetByID(ctx context.Context, id int) (*models.AdminUser, error) {
	query := `
		SELECT id, username, password_hash, nickname, status, created_at, updated_at
		FROM admin_user
		WHERE id = ?
	`

	var admin models.AdminUser
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&admin); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query admin by id: %w", err)
	}

	return &admin, nil
}
