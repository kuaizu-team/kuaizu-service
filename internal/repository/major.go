package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/api"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// MajorRepository handles major database operations
type MajorRepository struct {
	db *sqlx.DB
}

// NewMajorRepository creates a new MajorRepository
func NewMajorRepository(db *sqlx.DB) *MajorRepository {
	return &MajorRepository{db: db}
}

func (r *MajorRepository) List(ctx context.Context, params *api.ListMajorsParams) ([]models.Major, error) {
	query := `
		SELECT id, major_name, class_id
		FROM major
		WHERE class_id = ?
	`

	var majors []models.Major
	if err := r.db.SelectContext(ctx, &majors, query, params.ClassId); err != nil {
		return nil, fmt.Errorf("query majors: %w", err)
	}
	return majors, nil
}

// ListWithMajors returns major classes with their majors
func (r *MajorRepository) ListWithMajors(ctx context.Context, params api.ListMajorsParams) ([]models.MajorClass, error) {
	// 1. Query classes
	var conditions []string
	var args []interface{}

	if params.ClassId != nil {
		conditions = append(conditions, "id = ?")
		args = append(args, *params.ClassId)
	}

	if params.ClassKeyword != nil && *params.ClassKeyword != "" {
		conditions = append(conditions, "class_name LIKE ?")
		args = append(args, "%"+*params.ClassKeyword+"%")
	}

	classQuery := "SELECT id, class_name FROM major_class"
	if len(conditions) > 0 {
		classQuery += " WHERE " + strings.Join(conditions, " AND ")
	}
	classQuery += " ORDER BY id"

	var classes []models.MajorClass
	if err := r.db.SelectContext(ctx, &classes, classQuery, args...); err != nil {
		return nil, fmt.Errorf("query major classes: %w", err)
	}

	if len(classes) == 0 {
		return classes, nil
	}

	// 2. Query majors for these classes
	classIDs := make([]int, len(classes))
	for i, c := range classes {
		classIDs[i] = c.Id
	}

	// Use sqlx.In to expand the slice to placeholders
	majorQuery := `SELECT id, major_name, class_id FROM major WHERE class_id IN (?)`
	majorArgs := []interface{}{classIDs}

	if params.MajorKeyword != nil && *params.MajorKeyword != "" {
		majorQuery = `SELECT id, major_name, class_id FROM major WHERE class_id IN (?) AND major_name LIKE ?`
		majorArgs = append(majorArgs, "%"+*params.MajorKeyword+"%")
	}

	// Expand IN clause
	majorQuery, majorArgs, err := sqlx.In(majorQuery, majorArgs...)
	if err != nil {
		return nil, fmt.Errorf("expand IN clause: %w", err)
	}
	majorQuery = r.db.Rebind(majorQuery)
	majorQuery += " ORDER BY class_id, id"

	var majors []models.Major
	if err := r.db.SelectContext(ctx, &majors, majorQuery, majorArgs...); err != nil {
		return nil, fmt.Errorf("query majors: %w", err)
	}

	// Use map to easily append majors to classes
	classIDMap := make(map[int]*models.MajorClass)
	for i := range classes {
		classIDMap[classes[i].Id] = &classes[i]
	}

	for _, m := range majors {
		if mc, ok := classIDMap[m.ClassId]; ok {
			mc.Majors = append(mc.Majors, m)
		}
	}

	// 3. Filter out classes that have no majors (if filtering by major keyword)
	if params.MajorKeyword != nil && *params.MajorKeyword != "" {
		var filteredClasses []models.MajorClass
		for _, c := range classes {
			if len(c.Majors) > 0 {
				filteredClasses = append(filteredClasses, c)
			}
		}
		return filteredClasses, nil
	}

	return classes, nil
}
