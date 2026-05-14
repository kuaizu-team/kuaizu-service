package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// SchoolRepository handles school database operations
type SchoolRepository struct {
	db *sqlx.DB
}

// NewSchoolRepository creates a new SchoolRepository
func NewSchoolRepository(db *sqlx.DB) *SchoolRepository {
	return &SchoolRepository{db: db}
}

// SchoolListParams holds query parameters for listing schools.
type SchoolListParams struct {
	Keyword  *string // 关键词搜索（与地址参数互斥）
	Province *string // 省份
	City     *string // 城市
	District *string // 区县
}

// List retrieves schools by keyword (up to 30) or by exact province+city+district (all).
func (r *SchoolRepository) List(ctx context.Context, params SchoolListParams) ([]*models.School, error) {
	var query string
	var args []interface{}

	if params.Keyword != nil && *params.Keyword != "" {
		// Keyword search mode — limit 30
		pattern := "%" + *params.Keyword + "%"
		query = `
			SELECT id, school_name, school_code, province, city, district, created_at, updated_at
			FROM school
			WHERE school_name LIKE ?
			ORDER BY school_name ASC
			LIMIT 30
		`
		args = append(args, pattern)
	} else if params.Province != nil && params.City != nil && params.District != nil &&
		*params.Province != "" && *params.City != "" && *params.District != "" {
		// Location mode — exact match on province + city + district
		query = `
			SELECT id, school_name, school_code, province, city, district, created_at, updated_at
			FROM school
			WHERE province = ? AND city = ? AND district = ?
			ORDER BY school_name ASC
		`
		args = append(args, *params.Province, *params.City, *params.District)
	} else {
		// No valid filter — return empty
		return []*models.School{}, nil
	}

	var schools []*models.School
	if err := r.db.SelectContext(ctx, &schools, query, args...); err != nil {
		return nil, fmt.Errorf("query schools: %w", err)
	}
	return schools, nil
}

// ListProvinces returns a sorted distinct list of non-empty province values.
func (r *SchoolRepository) ListProvinces(ctx context.Context) ([]string, error) {
	query := `
		SELECT DISTINCT province
		FROM school
		WHERE province IS NOT NULL AND province != ''
		ORDER BY province ASC
	`
	var provinces []string
	if err := r.db.SelectContext(ctx, &provinces, query); err != nil {
		return nil, fmt.Errorf("query provinces: %w", err)
	}
	return provinces, nil
}

// ListCities returns a sorted distinct list of non-empty city values within a province.
func (r *SchoolRepository) ListCities(ctx context.Context, province string) ([]string, error) {
	query := `
		SELECT DISTINCT city
		FROM school
		WHERE province = ?
		  AND city IS NOT NULL AND city != ''
		ORDER BY city ASC
	`
	var cities []string
	if err := r.db.SelectContext(ctx, &cities, query, province); err != nil {
		return nil, fmt.Errorf("query cities: %w", err)
	}
	return cities, nil
}

// ListDistricts returns a sorted distinct list of non-empty district values within province+city.
func (r *SchoolRepository) ListDistricts(ctx context.Context, province, city string) ([]string, error) {
	query := `
		SELECT DISTINCT district
		FROM school
		WHERE province = ? AND city = ?
		  AND district IS NOT NULL AND district != '' AND district != '[]'
		ORDER BY district ASC
	`
	var districts []string
	if err := r.db.SelectContext(ctx, &districts, query, province, city); err != nil {
		return nil, fmt.Errorf("query districts: %w", err)
	}
	return districts, nil
}
