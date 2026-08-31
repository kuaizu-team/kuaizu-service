package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// RoleTagRepository provides read-only search access to the curated role-tag library.
type RoleTagRepository struct {
	db *sqlx.DB
}

func NewRoleTagRepository(db *sqlx.DB) *RoleTagRepository {
	return &RoleTagRepository{db: db}
}

func roleTagLikePattern(keyword string, prefix bool) string {
	escaped := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(keyword)
	if prefix {
		return escaped + "%"
	}
	return "%" + escaped + "%"
}

// Search returns prefix matches first, followed by substring matches, with a strict result cap.
func (r *RoleTagRepository) Search(ctx context.Context, keyword, roleCode string, limit int) ([]models.RoleTag, error) {
	keyword = strings.TrimSpace(keyword)
	roleCode = strings.TrimSpace(roleCode)
	if limit < 1 || limit > 20 {
		limit = 10
	}

	conditions := []string{"t.status = 1"}
	baseArgs := []interface{}{}
	join := ""
	if roleCode != "" {
		join = "INNER JOIN talent_role_tag_relation tr ON tr.tag_id = t.id"
		conditions = append(conditions, "tr.role_code = ?")
		baseArgs = append(baseArgs, roleCode)
	}

	whereClause := strings.Join(conditions, " AND ")
	query := ""
	args := []interface{}{}
	if keyword == "" {
		query = fmt.Sprintf(`
			SELECT DISTINCT t.id, t.tag_text, t.emoji, CONCAT(t.emoji, ' ', t.tag_text) AS display_text
			FROM talent_role_tag t
			%s
			WHERE %s
			ORDER BY t.sort_order, t.id
			LIMIT ?
		`, join, whereClause)
		args = append(args, baseArgs...)
		args = append(args, limit)
	} else {
		prefixPattern := roleTagLikePattern(keyword, true)
		containsPattern := roleTagLikePattern(keyword, false)
		query = fmt.Sprintf(`
			SELECT id, tag_text, emoji, display_text
			FROM (
				SELECT DISTINCT t.id, t.tag_text, t.emoji,
					CONCAT(t.emoji, ' ', t.tag_text) AS display_text,
					t.sort_order, 0 AS match_priority
				FROM talent_role_tag t
				%s
				WHERE %s AND t.tag_text LIKE ? ESCAPE '!'
				UNION ALL
				SELECT DISTINCT t.id, t.tag_text, t.emoji,
					CONCAT(t.emoji, ' ', t.tag_text) AS display_text,
					t.sort_order, 1 AS match_priority
				FROM talent_role_tag t
				%s
				WHERE %s
					AND t.tag_text LIKE ? ESCAPE '!'
					AND t.tag_text NOT LIKE ? ESCAPE '!'
			) matches
			ORDER BY match_priority, sort_order, id
			LIMIT ?
		`, join, whereClause, join, whereClause)
		args = append(args, baseArgs...)
		args = append(args, prefixPattern)
		args = append(args, baseArgs...)
		args = append(args, containsPattern, prefixPattern, limit)
	}

	var tags []models.RoleTag
	if err := r.db.SelectContext(ctx, &tags, query, args...); err != nil {
		return nil, fmt.Errorf("search role tags: %w", err)
	}
	return tags, nil
}
