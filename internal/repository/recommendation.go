package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

type RecommendationRepository struct {
	db *sqlx.DB
}

type ProjectRecommendationListParams struct {
	VisibleOnly  bool
	FeaturedOnly bool
	SchoolID     *int
	Limit        int
}

type ArticleRecommendationListParams struct {
	VisibleOnly  bool
	FeaturedOnly bool
	Limit        int
}

func NewRecommendationRepository(db *sqlx.DB) *RecommendationRepository {
	return &RecommendationRepository{db: db}
}

func (r *RecommendationRepository) ListProjects(ctx context.Context, params ProjectRecommendationListParams) ([]models.ProjectRecommendation, error) {
	conditions := []string{"p.deleted_at IS NULL", "p.status = ?"}
	args := []interface{}{models.ProjectStatusApproved}
	if params.VisibleOnly {
		conditions = append(conditions, "pr.is_visible = 1")
	}
	if params.FeaturedOnly {
		conditions = append(conditions, "pr.is_featured = 1")
	}
	limit := normalizeRecommendationLimit(params.Limit)
	query := fmt.Sprintf(`
		SELECT
			pr.id, pr.project_id, pr.description, pr.display_order, pr.is_visible, pr.is_featured, pr.interview_url, pr.created_at, pr.updated_at,
			p.id, p.creator_id, p.name, p.description, p.school_id, p.direction, p.member_count, p.status,
			p.promotion_status, p.promotion_expire_time, p.view_count, p.created_at, p.updated_at, p.reject_reason,
			p.deleted_at, p.is_cross_school, p.education_requirement, p.skill_requirement, p.publisher_role,
			p.initiating_school_id, s.school_name, pr_role.name AS publisher_role_name, ins.school_name AS initiating_school_name
		FROM project_recommendation pr
		JOIN project p ON p.id = pr.project_id
		LEFT JOIN school s ON p.school_id = s.id
		LEFT JOIN project_role pr_role ON p.publisher_role = pr_role.code
		LEFT JOIN school ins ON p.initiating_school_id = ins.id
		WHERE %s
		ORDER BY pr.display_order DESC, pr.created_at DESC, pr.id DESC
		LIMIT ?`, strings.Join(conditions, " AND "))
	args = append(args, limit)
	rows, err := r.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query project recommendations: %w", err)
	}
	defer rows.Close()
	items := make([]models.ProjectRecommendation, 0)
	for rows.Next() {
		var item models.ProjectRecommendation
		var project models.Project
		if err := rows.Scan(
			&item.ID, &item.ProjectID, &item.Description, &item.DisplayOrder, &item.IsVisible, &item.IsFeatured, &item.InterviewURL, &item.CreatedAt, &item.UpdatedAt,
			&project.ID, &project.CreatorID, &project.Name, &project.Description, &project.SchoolID, &project.Direction, &project.MemberCount, &project.Status,
			&project.PromotionStatus, &project.PromotionExpireTime, &project.ViewCount, &project.CreatedAt, &project.UpdatedAt, &project.RejectReason,
			&project.DeletedAt, &project.IsCrossSchool, &project.EducationRequirement, &project.SkillRequirement, &project.PublisherRole,
			&project.InitiatingSchoolID, &project.SchoolName, &project.PublisherRoleName, &project.InitiatingSchoolName,
		); err != nil {
			return nil, err
		}
		if params.SchoolID != nil && project.SchoolID != nil && *params.SchoolID == *project.SchoolID {
			item.IsFromMySchool = true
		}
		item.Project = &project
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.enrichRecommendedProjects(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *RecommendationRepository) GetProjectRecommendation(ctx context.Context, id int) (*models.ProjectRecommendation, error) {
	items, err := r.listProjectsByCondition(ctx, "pr.id = ?", []interface{}{id}, ProjectRecommendationListParams{Limit: 1})
	if err != nil || len(items) == 0 {
		return nil, err
	}
	return &items[0], nil
}

func (r *RecommendationRepository) listProjectsByCondition(ctx context.Context, extra string, extraArgs []interface{}, params ProjectRecommendationListParams) ([]models.ProjectRecommendation, error) {
	conditions := []string{"p.deleted_at IS NULL", extra}
	args := append([]interface{}{}, extraArgs...)
	limit := normalizeRecommendationLimit(params.Limit)
	query := fmt.Sprintf(`
		SELECT
			pr.id, pr.project_id, pr.description, pr.display_order, pr.is_visible, pr.is_featured, pr.interview_url, pr.created_at, pr.updated_at,
			p.id, p.creator_id, p.name, p.description, p.school_id, p.direction, p.member_count, p.status,
			p.promotion_status, p.promotion_expire_time, p.view_count, p.created_at, p.updated_at, p.reject_reason,
			p.deleted_at, p.is_cross_school, p.education_requirement, p.skill_requirement, p.publisher_role,
			p.initiating_school_id, s.school_name, pr_role.name AS publisher_role_name, ins.school_name AS initiating_school_name
		FROM project_recommendation pr
		JOIN project p ON p.id = pr.project_id
		LEFT JOIN school s ON p.school_id = s.id
		LEFT JOIN project_role pr_role ON p.publisher_role = pr_role.code
		LEFT JOIN school ins ON p.initiating_school_id = ins.id
		WHERE %s
		ORDER BY pr.display_order DESC, pr.created_at DESC, pr.id DESC
		LIMIT ?`, strings.Join(conditions, " AND "))
	args = append(args, limit)
	rows, err := r.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query project recommendation: %w", err)
	}
	defer rows.Close()
	items := make([]models.ProjectRecommendation, 0)
	for rows.Next() {
		var item models.ProjectRecommendation
		var project models.Project
		if err := rows.Scan(
			&item.ID, &item.ProjectID, &item.Description, &item.DisplayOrder, &item.IsVisible, &item.IsFeatured, &item.InterviewURL, &item.CreatedAt, &item.UpdatedAt,
			&project.ID, &project.CreatorID, &project.Name, &project.Description, &project.SchoolID, &project.Direction, &project.MemberCount, &project.Status,
			&project.PromotionStatus, &project.PromotionExpireTime, &project.ViewCount, &project.CreatedAt, &project.UpdatedAt, &project.RejectReason,
			&project.DeletedAt, &project.IsCrossSchool, &project.EducationRequirement, &project.SkillRequirement, &project.PublisherRole,
			&project.InitiatingSchoolID, &project.SchoolName, &project.PublisherRoleName, &project.InitiatingSchoolName,
		); err != nil {
			return nil, err
		}
		item.Project = &project
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.enrichRecommendedProjects(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *RecommendationRepository) UpsertProject(ctx context.Context, item *models.ProjectRecommendation) error {
	query := `INSERT INTO project_recommendation (project_id, description, display_order, is_visible, is_featured, interview_url)
		VALUES (:project_id, :description, :display_order, :is_visible, :is_featured, :interview_url)
		ON DUPLICATE KEY UPDATE description=VALUES(description), display_order=VALUES(display_order), is_visible=VALUES(is_visible), is_featured=VALUES(is_featured), interview_url=VALUES(interview_url), updated_at=CURRENT_TIMESTAMP`
	result, err := r.db.NamedExecContext(ctx, query, item)
	if err != nil {
		return fmt.Errorf("upsert project recommendation: %w", err)
	}
	if id, err := result.LastInsertId(); err == nil && id > 0 {
		item.ID = int(id)
	}
	return nil
}

func (r *RecommendationRepository) UpdateProject(ctx context.Context, item *models.ProjectRecommendation) error {
	result, err := r.db.NamedExecContext(ctx, `UPDATE project_recommendation SET project_id=:project_id, description=:description, display_order=:display_order, is_visible=:is_visible, is_featured=:is_featured, interview_url=:interview_url, updated_at=CURRENT_TIMESTAMP WHERE id=:id`, item)
	if err != nil {
		return fmt.Errorf("update project recommendation: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *RecommendationRepository) DeleteProject(ctx context.Context, id int) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM project_recommendation WHERE id = ?", id)
	return err
}

func (r *RecommendationRepository) ListArticles(ctx context.Context, kind string, params ArticleRecommendationListParams) ([]models.ArticleRecommendation, error) {
	table, err := recommendationArticleTable(kind)
	if err != nil {
		return nil, err
	}
	conditions := []string{"1=1"}
	args := []interface{}{}
	if params.VisibleOnly {
		conditions = append(conditions, "is_visible = 1")
	}
	if params.FeaturedOnly {
		conditions = append(conditions, "is_featured = 1")
	}
	limit := normalizeRecommendationLimit(params.Limit)
	query := fmt.Sprintf(`SELECT id, title, description, article_url, display_order, is_visible, is_featured, created_at, updated_at FROM %s WHERE %s ORDER BY display_order DESC, created_at DESC, id DESC LIMIT ?`, table, strings.Join(conditions, " AND "))
	args = append(args, limit)
	var items []models.ArticleRecommendation
	if err := r.db.SelectContext(ctx, &items, query, args...); err != nil {
		return nil, fmt.Errorf("query %s recommendations: %w", kind, err)
	}
	return items, nil
}

func (r *RecommendationRepository) GetArticle(ctx context.Context, kind string, id int) (*models.ArticleRecommendation, error) {
	table, err := recommendationArticleTable(kind)
	if err != nil {
		return nil, err
	}
	var item models.ArticleRecommendation
	query := fmt.Sprintf(`SELECT id, title, description, article_url, display_order, is_visible, is_featured, created_at, updated_at FROM %s WHERE id = ?`, table)
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&item); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query %s recommendation: %w", kind, err)
	}
	return &item, nil
}

func (r *RecommendationRepository) CreateArticle(ctx context.Context, kind string, item *models.ArticleRecommendation) error {
	table, err := recommendationArticleTable(kind)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`INSERT INTO %s (title, description, article_url, display_order, is_visible, is_featured) VALUES (:title, :description, :article_url, :display_order, :is_visible, :is_featured)`, table)
	result, err := r.db.NamedExecContext(ctx, query, item)
	if err != nil {
		return fmt.Errorf("create %s recommendation: %w", kind, err)
	}
	id, _ := result.LastInsertId()
	item.ID = int(id)
	return nil
}

func (r *RecommendationRepository) UpdateArticle(ctx context.Context, kind string, item *models.ArticleRecommendation) error {
	table, err := recommendationArticleTable(kind)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`UPDATE %s SET title=:title, description=:description, article_url=:article_url, display_order=:display_order, is_visible=:is_visible, is_featured=:is_featured, updated_at=CURRENT_TIMESTAMP WHERE id=:id`, table)
	result, err := r.db.NamedExecContext(ctx, query, item)
	if err != nil {
		return fmt.Errorf("update %s recommendation: %w", kind, err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *RecommendationRepository) DeleteArticle(ctx context.Context, kind string, id int) error {
	table, err := recommendationArticleTable(kind)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = ?", table), id)
	return err
}

func (r *RecommendationRepository) enrichRecommendedProjects(ctx context.Context, items []models.ProjectRecommendation) error {
	if len(items) == 0 {
		return nil
	}
	projects := make([]models.Project, 0, len(items))
	index := make(map[int][]int, len(items))
	ids := make([]int, 0, len(items))
	for i := range items {
		if items[i].Project == nil {
			continue
		}
		ids = append(ids, items[i].Project.ID)
		projects = append(projects, *items[i].Project)
		index[items[i].Project.ID] = append(index[items[i].Project.ID], i)
	}
	if len(ids) == 0 {
		return nil
	}
	if err := r.enrichProjectTags(ctx, projects); err != nil {
		return err
	}
	events, err := r.listEventsByProjectIDs(ctx, ids)
	if err != nil {
		return err
	}
	for i := range projects {
		projects[i].Events = events[projects[i].ID]
		for _, itemIndex := range index[projects[i].ID] {
			project := projects[i]
			items[itemIndex].Project = &project
		}
	}
	return nil
}

func (r *RecommendationRepository) enrichProjectTags(ctx context.Context, projects []models.Project) error {
	ids := make([]int, len(projects))
	index := make(map[int]int, len(projects))
	for i := range projects {
		ids[i] = projects[i].ID
		index[projects[i].ID] = i
	}
	query, args, err := sqlx.In(`SELECT r.project_id,t.id,t.name FROM project_tag_relation r JOIN project_tag t ON t.id=r.tag_id WHERE r.project_id IN (?) AND t.status=1 ORDER BY t.sort_order,t.id`, ids)
	if err != nil {
		return err
	}
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return fmt.Errorf("query recommendation project tags: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var projectID int
		var tag models.ProjectTag
		if err := rows.Scan(&projectID, &tag.ID, &tag.Name); err != nil {
			return err
		}
		if i, ok := index[projectID]; ok {
			projects[i].Tags = append(projects[i].Tags, tag)
		}
	}
	return rows.Err()
}

func (r *RecommendationRepository) listEventsByProjectIDs(ctx context.Context, projectIDs []int) (map[int][]models.Event, error) {
	result := map[int][]models.Event{}
	query, args, err := sqlx.In(`SELECT pe.project_id, e.id, e.name, e.is_ranking, e.registration_deadline, e.article_url, e.organizer_name, e.description, e.resource_url, e.qq_group, e.allow_cross_school, e.allow_cross_major, e.view_count, e.display_order, e.created_at, e.updated_at
		FROM project_event pe JOIN event e ON e.id = pe.event_id WHERE pe.project_id IN (?) ORDER BY e.display_order DESC, e.created_at DESC, e.id DESC`, projectIDs)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("query recommendation project events: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var projectID int
		var event models.Event
		if err := rows.Scan(&projectID, &event.ID, &event.Name, &event.IsRanking, &event.RegistrationDeadline, &event.ArticleURL, &event.OrganizerName, &event.Description, &event.ResourceURL, &event.QQGroup, &event.AllowCrossSchool, &event.AllowCrossMajor, &event.ViewCount, &event.DisplayOrder, &event.CreatedAt, &event.UpdatedAt); err != nil {
			return nil, err
		}
		result[projectID] = append(result[projectID], event)
	}
	return result, rows.Err()
}

func recommendationArticleTable(kind string) (string, error) {
	switch kind {
	case "podcasts":
		return "podcast_recommendation", nil
	case "news":
		return "news_recommendation", nil
	default:
		return "", fmt.Errorf("invalid recommendation kind")
	}
}

func normalizeRecommendationLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 100
	}
	return limit
}
