package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// Tag scores are lexicographic: exact overlap, then role similarity. Custom
// tags participate in exact overlap only; generic all-role tags add no roles.
type tagMatchScore struct {
	Exact int `json:"exact"`
	Role  int `json:"role"`
	Count int `json:"count"`
}

type matchingTag struct {
	Text  string `db:"tag_text"`
	Emoji string `db:"emoji"`
	Role  string `db:"role_code"`
}

type tagMatcher struct {
	aliases map[string]string
	roles   map[string]map[string]bool
}

func normalizedTag(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func newTagMatcher(tags []matchingTag, roleCount int) *tagMatcher {
	m := &tagMatcher{aliases: map[string]string{}, roles: map[string]map[string]bool{}}
	for _, t := range tags {
		key := normalizedTag(t.Text)
		m.aliases[normalizedTag(t.Emoji+" "+t.Text)] = key
		m.aliases[normalizedTag(t.Emoji+t.Text)] = key
		if m.roles[key] == nil {
			m.roles[key] = map[string]bool{}
		}
		if t.Role != "" {
			m.roles[key][t.Role] = true
		}
	}
	for key, roles := range m.roles {
		if roleCount > 0 && len(roles) == roleCount {
			delete(m.roles, key)
		}
	}
	return m
}

func loadTagMatcher(ctx context.Context, db *sqlx.DB) (*tagMatcher, error) {
	var tags []matchingTag
	err := db.SelectContext(ctx, &tags, `SELECT t.tag_text, t.emoji, COALESCE(pr.code, '') AS role_code
 FROM talent_role_tag t
 LEFT JOIN talent_role_tag_relation tr ON tr.tag_id = t.id
 LEFT JOIN project_role pr ON pr.code = tr.role_code AND pr.status = 1
 WHERE t.status = 1`)
	if err != nil {
		return nil, err
	}
	var roleCount int
	if err := db.GetContext(ctx, &roleCount, `SELECT COUNT(*) FROM project_role WHERE status = 1`); err != nil {
		return nil, err
	}
	return newTagMatcher(tags, roleCount), nil
}

func (m *tagMatcher) keys(tags []string) map[string]bool {
	result := map[string]bool{}
	for _, t := range tags {
		key := normalizedTag(t)
		if alias, ok := m.aliases[key]; ok {
			key = alias
		}
		if key != "" {
			result[key] = true
		}
	}
	return result
}

func (m *tagMatcher) roleVector(keys map[string]bool) map[string]float64 {
	v := map[string]float64{}
	for key := range keys {
		roles := m.roles[key]
		for role := range roles {
			v[role] += 1 / float64(len(roles))
		}
	}
	return v
}

type tagMatchBasis struct {
	keys  map[string]bool
	roles map[string]float64
	norm  float64
}

func (m *tagMatcher) basis(tags []string) tagMatchBasis {
	keys := m.keys(tags)
	roles := m.roleVector(keys)
	var norm float64
	for _, v := range roles {
		norm += v * v
	}
	return tagMatchBasis{keys: keys, roles: roles, norm: norm}
}

func (m *tagMatcher) score(b tagMatchBasis, tags []string) tagMatchScore {
	keys := m.keys(tags)
	result := tagMatchScore{Count: len(keys)}
	for key := range keys {
		if b.keys[key] {
			result.Exact++
		}
	}
	var dot, norm float64
	for role, value := range m.roleVector(keys) {
		dot += value * b.roles[role]
		norm += value * value
	}
	if norm > 0 && b.norm > 0 {
		result.Role = int(math.Round(1000000 * dot / math.Sqrt(norm*b.norm)))
	}
	return result
}

func viewerProfileTags(ctx context.Context, db *sqlx.DB, userID int) ([]string, error) {
	var tags models.JSONStringArray
	err := db.GetContext(ctx, &tags, `SELECT skill_summary FROM talent_profile WHERE user_id = ?`, userID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return tags.Items, err
}

// A published but tagless project deliberately prevents the profile fallback.
func talentReferenceTags(ctx context.Context, db *sqlx.DB, userID int) ([]string, error) {
	var rows []struct {
		Name *string `db:"name"`
	}
	err := db.SelectContext(ctx, &rows, `SELECT t.name FROM project p
 LEFT JOIN project_tag_relation tr ON tr.project_id = p.id
 LEFT JOIN project_tag t ON t.id = tr.tag_id AND t.status = 1
 WHERE p.creator_id = ? AND p.deleted_at IS NULL AND p.status IN (1, 3, 5)`, userID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return viewerProfileTags(ctx, db, userID)
	}
	var tags []string
	for _, row := range rows {
		if row.Name != nil {
			tags = append(tags, *row.Name)
		}
	}
	return tags, nil
}

func (r *ProjectRepository) scoreProjectTags(ctx context.Context, candidates []projectRankCandidate, viewerID *int) error {
	if viewerID == nil || *viewerID <= 0 || len(candidates) == 0 {
		return nil
	}
	tags, err := viewerProfileTags(ctx, r.db, *viewerID)
	if err != nil {
		return err
	}
	if len(tags) == 0 {
		return nil
	}
	matcher, err := loadTagMatcher(ctx, r.db)
	if err != nil {
		return err
	}
	basis := matcher.basis(tags)
	if len(basis.keys) == 0 {
		return nil
	}
	// Batch only candidate tags, never a query per project.
	ids := make([]int, len(candidates))
	for i := range candidates {
		ids[i] = candidates[i].ID
	}
	query, args, err := sqlx.In(`SELECT tr.project_id, t.name FROM project_tag_relation tr
 JOIN project_tag t ON t.id = tr.tag_id AND t.status = 1 WHERE tr.project_id IN (?)`, ids)
	if err != nil {
		return err
	}
	var rows []struct {
		ID   int    `db:"project_id"`
		Name string `db:"name"`
	}
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return err
	}
	byProject := map[int][]string{}
	for _, row := range rows {
		byProject[row.ID] = append(byProject[row.ID], row.Name)
	}
	for i := range candidates {
		candidates[i].TagScore = matcher.score(basis, byProject[candidates[i].ID])
	}
	return nil
}

// Return one JSON parameter for a relational join, not an unbounded CASE clause.
// Only filtered candidates' IDs and tag arrays are loaded; pagination stays in SQL.
func (r *TalentProfileRepository) talentTagScores(ctx context.Context, userID int, where string, args []interface{}) (string, bool, error) {
	tags, err := talentReferenceTags(ctx, r.db, userID)
	if err != nil {
		return "", false, err
	}
	matcher, err := loadTagMatcher(ctx, r.db)
	if err != nil {
		return "", false, err
	}
	basis := matcher.basis(tags)
	var candidates []struct {
		ID   int                    `db:"id"`
		Tags models.JSONStringArray `db:"skill_summary"`
	}
	query := "SELECT tp.id, tp.skill_summary FROM talent_profile tp LEFT JOIN `user` u ON tp.user_id = u.id WHERE " + where
	if err := r.db.SelectContext(ctx, &candidates, query, args...); err != nil {
		return "", false, err
	}
	type scoredTalent struct {
		ID int `json:"id"`
		tagMatchScore
	}
	scores := make([]scoredTalent, 0, len(candidates))
	for _, c := range candidates {
		scores = append(scores, scoredTalent{ID: c.ID, tagMatchScore: matcher.score(basis, c.Tags.Items)})
	}
	data, err := json.Marshal(scores)
	if err != nil {
		return "", false, fmt.Errorf("marshal talent tag scores: %w", err)
	}
	return string(data), len(basis.keys) == 0, nil
}
