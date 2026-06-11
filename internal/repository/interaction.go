package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/oss"
)

type FavoriteProject struct {
	models.Project
	FavoritedAt time.Time `db:"favorited_at"`
}

type FavoriteTalent struct {
	models.TalentProfile
	FavoritedAt time.Time `db:"favorited_at"`
}

type InteractionUnread struct {
	LikeCount     int `db:"like_count" json:"like_count"`
	FavoriteCount int `db:"favorite_count" json:"favorite_count"`
	ShareCount    int `db:"share_count" json:"share_count"`
	VisitCount    int `db:"visit_count" json:"visit_count"`
	TotalCount    int `db:"total_count" json:"total_count"`
}

type DashboardUnreadTotals struct {
	ProjectCount int `db:"project_count" json:"projectCount"`
	TalentCount  int `db:"talent_count" json:"talentCount"`
	TotalCount   int `db:"total_count" json:"totalCount"`
}

const (
	InteractionProject = "projects"
	InteractionTalent  = "talent-profiles"
)

type InteractionRepository struct{ db *sqlx.DB }

func NewInteractionRepository(db *sqlx.DB) *InteractionRepository {
	return &InteractionRepository{db: db}
}

func interactionTables(target string) (like, favorite, share, idColumn string, err error) {
	switch target {
	case InteractionProject:
		return "project_like", "project_favorite", "project_share", "project_id", nil
	case InteractionTalent:
		return "talent_like", "talent_favorite", "talent_share", "talent_profile_id", nil
	default:
		return "", "", "", "", fmt.Errorf("invalid interaction target")
	}
}

func (r *InteractionRepository) Get(ctx context.Context, target string, targetID, userID int) (*models.Interaction, error) {
	return getInteraction(ctx, r.db, target, targetID, userID)
}

func getInteraction(ctx context.Context, db sqlx.ExtContext, target string, targetID, userID int) (*models.Interaction, error) {
	like, favorite, share, idCol, err := interactionTables(target)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT
		EXISTS(SELECT 1 FROM %s WHERE %s=? AND user_id=?) liked,
		EXISTS(SELECT 1 FROM %s WHERE %s=? AND user_id=?) favorited,
		(SELECT COUNT(*) FROM %s WHERE %s=?) like_count,
		(SELECT COUNT(*) FROM %s WHERE %s=?) favorite_count,
		(SELECT COUNT(*) FROM %s WHERE %s=?) share_count`,
		like, idCol, favorite, idCol, like, idCol, favorite, idCol, share, idCol)
	var result models.Interaction
	err = sqlx.GetContext(ctx, db, &result, query, targetID, userID, targetID, userID, targetID, targetID, targetID)
	return &result, err
}

func (r *InteractionRepository) Toggle(ctx context.Context, target, kind string, targetID, userID int) (*models.Interaction, error) {
	like, favorite, _, idCol, err := interactionTables(target)
	if err != nil {
		return nil, err
	}
	table := like
	if kind == "favorite" {
		table = favorite
	} else if kind != "like" {
		return nil, fmt.Errorf("invalid interaction kind")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s=? AND user_id=?", table, idCol), targetID, userID)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s,user_id) VALUES (?,?)", table, idCol), targetID, userID); err != nil {
			return nil, err
		}
	}
	current, err := getInteraction(ctx, tx, target, targetID, userID)
	if err != nil {
		return nil, err
	}
	active := current.Liked
	if kind == "favorite" {
		active = current.Favorited
	}
	current.Active = &active
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return current, nil
}

func (r *InteractionRepository) Share(ctx context.Context, target string, targetID, userID int, channel string) (*models.Interaction, error) {
	_, _, table, idCol, err := interactionTables(target)
	if err != nil {
		return nil, err
	}
	if _, err := r.db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (%s,user_id,channel) VALUES (?,?,?)", table, idCol), targetID, userID, channel); err != nil {
		return nil, err
	}
	return r.Get(ctx, target, targetID, userID)
}

func (r *InteractionRepository) Batch(ctx context.Context, target string, targetIDs []int, userID int) (map[int]models.Interaction, error) {
	result := make(map[int]models.Interaction, len(targetIDs))
	if len(targetIDs) == 0 {
		return result, nil
	}
	like, favorite, share, idCol, err := interactionTables(target)
	if err != nil {
		return nil, err
	}
	parts := make([]string, len(targetIDs))
	args := []interface{}{userID, userID}
	for i, id := range targetIDs {
		if i == 0 {
			parts[i] = "SELECT ? target_id"
		} else {
			parts[i] = "SELECT ?"
		}
		args = append(args, id)
	}
	query := fmt.Sprintf(`SELECT x.target_id,
		EXISTS(SELECT 1 FROM %s WHERE %s=x.target_id AND user_id=?) liked,
		EXISTS(SELECT 1 FROM %s WHERE %s=x.target_id AND user_id=?) favorited,
		(SELECT COUNT(*) FROM %s WHERE %s=x.target_id) like_count,
		(SELECT COUNT(*) FROM %s WHERE %s=x.target_id) favorite_count,
		(SELECT COUNT(*) FROM %s WHERE %s=x.target_id) share_count
		FROM (%s) x`,
		like, idCol, favorite, idCol, like, idCol, favorite, idCol, share, idCol, strings.Join(parts, " UNION ALL "))
	type row struct {
		TargetID int `db:"target_id"`
		models.Interaction
	}
	var rows []row
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return nil, err
	}
	for _, item := range rows {
		result[item.TargetID] = item.Interaction
	}
	return result, nil
}

func (r *InteractionRepository) CountsSince(ctx context.Context, target string, targetID, days int) (models.Interaction, error) {
	like, favorite, share, idCol, err := interactionTables(target)
	if err != nil {
		return models.Interaction{}, err
	}
	filter := ""
	args := []interface{}{targetID, targetID, targetID}
	if days > 0 {
		filter = " AND created_at>=DATE_SUB(NOW(),INTERVAL ? DAY)"
		args = []interface{}{targetID, days, targetID, days, targetID, days}
	}
	query := fmt.Sprintf(`SELECT
		(SELECT COUNT(*) FROM %s WHERE %s=?%s) like_count,
		(SELECT COUNT(*) FROM %s WHERE %s=?%s) favorite_count,
		(SELECT COUNT(*) FROM %s WHERE %s=?%s) share_count`,
		like, idCol, filter, favorite, idCol, filter, share, idCol, filter)
	var result models.Interaction
	err = r.db.QueryRowxContext(ctx, query, args...).StructScan(&result)
	return result, err
}

func (r *InteractionRepository) ListUsers(ctx context.Context, target, kind string, targetID, page, size, days int) ([]models.InteractionUser, int64, error) {
	like, favorite, share, idCol, err := interactionTables(target)
	if err != nil {
		return nil, 0, err
	}
	table := map[string]string{"like": like, "favorite": favorite, "share": share}[kind]
	if table == "" {
		return nil, 0, fmt.Errorf("invalid interaction kind")
	}
	where, args := "i."+idCol+"=?", []interface{}{targetID}
	if days > 0 {
		where += " AND i.created_at>=DATE_SUB(NOW(),INTERVAL ? DAY)"
		args = append(args, days)
	}
	var total int64
	if err := r.db.QueryRowxContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s i WHERE i.%s=?", table, idCol), targetID).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := fmt.Sprintf(`SELECT i.user_id,tp.id talent_profile_id,u.nickname,u.avatar_url,i.created_at operated_at
		FROM %s i JOIN `+"`user`"+` u ON u.id=i.user_id LEFT JOIN talent_profile tp ON tp.user_id=u.id
		WHERE %s ORDER BY i.created_at DESC LIMIT ? OFFSET ?`, table, where)
	args = append(args, size, (page-1)*size)
	var users []models.InteractionUser
	err = r.db.SelectContext(ctx, &users, query, args...)
	if err != nil {
		return nil, 0, err
	}
	for i := range users {
		if users[i].AvatarURL != nil && *users[i].AvatarURL != "" {
			fullURL := oss.FullURL(*users[i].AvatarURL)
			users[i].AvatarURL = &fullURL
		}
	}
	return users, total, err
}

func (r *InteractionRepository) UnreadFavorites(ctx context.Context, userID int) (models.FavoriteViewState, error) {
	var state models.FavoriteViewState
	query := `SELECT
		(SELECT COUNT(*) FROM project_favorite f LEFT JOIN user_favorite_view_state s ON s.user_id=f.user_id AND s.target_type='projects' WHERE f.user_id=? AND f.created_at>COALESCE(s.last_viewed_at,'1970-01-01')),
		(SELECT COUNT(*) FROM talent_favorite f LEFT JOIN user_favorite_view_state s ON s.user_id=f.user_id AND s.target_type='talent-profiles' WHERE f.user_id=? AND f.created_at>COALESCE(s.last_viewed_at,'1970-01-01'))`
	err := r.db.QueryRowxContext(ctx, query, userID, userID).Scan(&state.Projects, &state.TalentProfiles)
	state.Total = state.Projects + state.TalentProfiles
	state.ProjectCount = state.Projects
	state.TalentCount = state.TalentProfiles
	state.TotalCount = state.Total
	return state, err
}

func (r *InteractionRepository) MarkFavoritesViewed(ctx context.Context, userID int, target string) error {
	if target != InteractionProject && target != InteractionTalent {
		return fmt.Errorf("invalid interaction target")
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO user_favorite_view_state(user_id,target_type,last_viewed_at)
		VALUES(?,?,NOW()) ON DUPLICATE KEY UPDATE last_viewed_at=VALUES(last_viewed_at),updated_at=NOW()`, userID, target)
	return err
}

func (r *InteractionRepository) UnreadForTarget(ctx context.Context, target string, targetID, ownerUserID int) (InteractionUnread, error) {
	like, favorite, share, idCol, err := interactionTables(target)
	if err != nil {
		return InteractionUnread{}, err
	}
	query := fmt.Sprintf(`SELECT
		(SELECT COUNT(*) FROM %s i WHERE i.%s=? AND i.created_at>COALESCE(
			(SELECT s.last_viewed_at FROM interaction_dashboard_view_state s WHERE s.user_id=? AND s.target_type=? AND s.target_id=? AND s.interaction_type='like'),
			'1970-01-01 00:00:01')) like_count,
		(SELECT COUNT(*) FROM %s i WHERE i.%s=? AND i.created_at>COALESCE(
			(SELECT s.last_viewed_at FROM interaction_dashboard_view_state s WHERE s.user_id=? AND s.target_type=? AND s.target_id=? AND s.interaction_type='favorite'),
			'1970-01-01 00:00:01')) favorite_count,
		(SELECT COUNT(*) FROM %s i WHERE i.%s=? AND i.created_at>COALESCE(
			(SELECT s.last_viewed_at FROM interaction_dashboard_view_state s WHERE s.user_id=? AND s.target_type=? AND s.target_id=? AND s.interaction_type='share'),
			'1970-01-01 00:00:01')) share_count`,
		like, idCol, favorite, idCol, share, idCol)
	args := []interface{}{
		targetID, ownerUserID, target, targetID,
		targetID, ownerUserID, target, targetID,
		targetID, ownerUserID, target, targetID,
	}
	var unread InteractionUnread
	if err := r.db.QueryRowxContext(ctx, query, args...).StructScan(&unread); err != nil {
		return InteractionUnread{}, err
	}
	unread.TotalCount = unread.LikeCount + unread.FavoriteCount + unread.ShareCount
	return unread, nil
}

func (r *InteractionRepository) UnreadDashboardTotals(ctx context.Context, ownerUserID int) (DashboardUnreadTotals, error) {
	query := `SELECT
		(
			(SELECT COUNT(*) FROM project_like i JOIN project p ON p.id=i.project_id WHERE p.creator_id=? AND i.created_at>COALESCE(
				(SELECT s.last_viewed_at FROM interaction_dashboard_view_state s WHERE s.user_id=? AND s.target_type='projects' AND s.target_id=i.project_id AND s.interaction_type='like'),'1970-01-01 00:00:01'))
			+(SELECT COUNT(*) FROM project_favorite i JOIN project p ON p.id=i.project_id WHERE p.creator_id=? AND i.created_at>COALESCE(
				(SELECT s.last_viewed_at FROM interaction_dashboard_view_state s WHERE s.user_id=? AND s.target_type='projects' AND s.target_id=i.project_id AND s.interaction_type='favorite'),'1970-01-01 00:00:01'))
			+(SELECT COUNT(*) FROM project_share i JOIN project p ON p.id=i.project_id WHERE p.creator_id=? AND i.created_at>COALESCE(
				(SELECT s.last_viewed_at FROM interaction_dashboard_view_state s WHERE s.user_id=? AND s.target_type='projects' AND s.target_id=i.project_id AND s.interaction_type='share'),'1970-01-01 00:00:01'))
		) project_count,
		(
			(SELECT COUNT(*) FROM talent_like i JOIN talent_profile tp ON tp.id=i.talent_profile_id WHERE tp.user_id=? AND i.created_at>COALESCE(
				(SELECT s.last_viewed_at FROM interaction_dashboard_view_state s WHERE s.user_id=? AND s.target_type='talent-profiles' AND s.target_id=i.talent_profile_id AND s.interaction_type='like'),'1970-01-01 00:00:01'))
			+(SELECT COUNT(*) FROM talent_favorite i JOIN talent_profile tp ON tp.id=i.talent_profile_id WHERE tp.user_id=? AND i.created_at>COALESCE(
				(SELECT s.last_viewed_at FROM interaction_dashboard_view_state s WHERE s.user_id=? AND s.target_type='talent-profiles' AND s.target_id=i.talent_profile_id AND s.interaction_type='favorite'),'1970-01-01 00:00:01'))
			+(SELECT COUNT(*) FROM talent_share i JOIN talent_profile tp ON tp.id=i.talent_profile_id WHERE tp.user_id=? AND i.created_at>COALESCE(
				(SELECT s.last_viewed_at FROM interaction_dashboard_view_state s WHERE s.user_id=? AND s.target_type='talent-profiles' AND s.target_id=i.talent_profile_id AND s.interaction_type='share'),'1970-01-01 00:00:01'))
		) talent_count`
	args := make([]interface{}, 0, 12)
	for i := 0; i < 12; i++ {
		args = append(args, ownerUserID)
	}
	var totals DashboardUnreadTotals
	if err := r.db.QueryRowxContext(ctx, query, args...).StructScan(&totals); err != nil {
		return DashboardUnreadTotals{}, err
	}
	totals.TotalCount = totals.ProjectCount + totals.TalentCount
	return totals, nil
}

func (r *InteractionRepository) BatchProjectUnread(ctx context.Context, ownerUserID int, projectIDs []int) (map[int]int, error) {
	result := make(map[int]int, len(projectIDs))
	if len(projectIDs) == 0 {
		return result, nil
	}
	query, args, err := sqlx.In(`SELECT x.project_id,SUM(x.cnt) unread_count FROM (
		SELECT i.project_id,COUNT(*) cnt FROM project_like i JOIN project p ON p.id=i.project_id
		WHERE p.creator_id=? AND i.project_id IN (?) AND i.created_at>COALESCE(
			(SELECT s.last_viewed_at FROM interaction_dashboard_view_state s WHERE s.user_id=? AND s.target_type='projects' AND s.target_id=i.project_id AND s.interaction_type='like'),'1970-01-01 00:00:01') GROUP BY i.project_id
		UNION ALL
		SELECT i.project_id,COUNT(*) cnt FROM project_favorite i JOIN project p ON p.id=i.project_id
		WHERE p.creator_id=? AND i.project_id IN (?) AND i.created_at>COALESCE(
			(SELECT s.last_viewed_at FROM interaction_dashboard_view_state s WHERE s.user_id=? AND s.target_type='projects' AND s.target_id=i.project_id AND s.interaction_type='favorite'),'1970-01-01 00:00:01') GROUP BY i.project_id
		UNION ALL
		SELECT i.project_id,COUNT(*) cnt FROM project_share i JOIN project p ON p.id=i.project_id
		WHERE p.creator_id=? AND i.project_id IN (?) AND i.created_at>COALESCE(
			(SELECT s.last_viewed_at FROM interaction_dashboard_view_state s WHERE s.user_id=? AND s.target_type='projects' AND s.target_id=i.project_id AND s.interaction_type='share'),'1970-01-01 00:00:01') GROUP BY i.project_id
	) x GROUP BY x.project_id`, ownerUserID, projectIDs, ownerUserID, ownerUserID, projectIDs, ownerUserID, ownerUserID, projectIDs, ownerUserID)
	if err != nil {
		return nil, err
	}
	type row struct {
		ProjectID   int `db:"project_id"`
		UnreadCount int `db:"unread_count"`
	}
	var rows []row
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
		return nil, err
	}
	for _, item := range rows {
		result[item.ProjectID] = item.UnreadCount
	}
	return result, nil
}

func (r *InteractionRepository) MarkDashboardViewed(ctx context.Context, ownerUserID int, target string, targetID int, kind *string) error {
	if _, _, _, _, err := interactionTables(target); err != nil {
		return err
	}
	kinds := []string{"like", "favorite", "share"}
	if kind != nil {
		kinds = []string{*kind}
	}
	values := make([]string, 0, len(kinds))
	args := make([]interface{}, 0, len(kinds)*4)
	for _, interactionType := range kinds {
		values = append(values, "(?,?,?,?,NOW())")
		args = append(args, ownerUserID, target, targetID, interactionType)
	}
	query := `INSERT INTO interaction_dashboard_view_state(user_id,target_type,target_id,interaction_type,last_viewed_at)
		VALUES ` + strings.Join(values, ",") + `
		ON DUPLICATE KEY UPDATE last_viewed_at=VALUES(last_viewed_at),updated_at=NOW()`
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *InteractionRepository) ListFavoriteProjects(ctx context.Context, userID, page, size int) ([]FavoriteProject, int64, error) {
	var total int64
	if err := r.db.QueryRowxContext(ctx, "SELECT COUNT(*) FROM project_favorite WHERE user_id=?", userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := `SELECT p.id,p.creator_id,p.name,p.description,p.school_id,p.direction,p.member_count,p.status,
		p.promotion_status,p.promotion_expire_time,p.view_count,p.created_at,p.updated_at,p.is_cross_school,
		p.education_requirement,p.skill_requirement,p.publisher_role,p.initiating_school_id,
		s.school_name,pr.name publisher_role_name,ins.school_name initiating_school_name,f.created_at favorited_at
		FROM project_favorite f JOIN project p ON p.id=f.project_id
		LEFT JOIN school s ON s.id=p.school_id LEFT JOIN project_role pr ON pr.code=p.publisher_role
		LEFT JOIN school ins ON ins.id=p.initiating_school_id
		WHERE f.user_id=? ORDER BY f.created_at DESC LIMIT ? OFFSET ?`
	var items []FavoriteProject
	err := r.db.SelectContext(ctx, &items, query, userID, size, (page-1)*size)
	if err == nil {
		err = r.enrichFavoriteProjects(ctx, items)
	}
	return items, total, err
}

func (r *InteractionRepository) enrichFavoriteProjects(ctx context.Context, items []FavoriteProject) error {
	if len(items) == 0 {
		return nil
	}
	ids, creatorIDs := make([]int, len(items)), make([]int, len(items))
	projectIndex, creatorIndex := map[int]int{}, map[int][]int{}
	for i := range items {
		ids[i], creatorIDs[i] = items[i].ID, items[i].CreatorID
		projectIndex[items[i].ID] = i
		creatorIndex[items[i].CreatorID] = append(creatorIndex[items[i].CreatorID], i)
	}
	tagQuery, tagArgs, _ := sqlx.In(`SELECT r.project_id,t.id,t.name FROM project_tag_relation r JOIN project_tag t ON t.id=r.tag_id WHERE r.project_id IN (?) AND t.status=1`, ids)
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(tagQuery), tagArgs...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var projectID int
		var tag models.ProjectTag
		if err := rows.Scan(&projectID, &tag.ID, &tag.Name); err != nil {
			rows.Close()
			return err
		}
		i := projectIndex[projectID]
		items[i].Tags = append(items[i].Tags, tag)
	}
	rows.Close()
	creatorQuery, creatorArgs, _ := sqlx.In(`SELECT u.id,u.openid,u.nickname,u.avatar_url,u.auth_status,s.school_name,m.major_name,tp.id talent_profile_id
		FROM `+"`user`"+` u LEFT JOIN school s ON s.id=u.school_id LEFT JOIN major m ON m.id=u.major_id LEFT JOIN talent_profile tp ON tp.user_id=u.id WHERE u.id IN (?)`, creatorIDs)
	type creatorRow struct{ models.User }
	var creators []creatorRow
	if err := r.db.SelectContext(ctx, &creators, r.db.Rebind(creatorQuery), creatorArgs...); err != nil {
		return err
	}
	for _, row := range creators {
		for _, i := range creatorIndex[row.ID] {
			creator := row.User
			items[i].Creator = &creator
		}
	}
	return nil
}

func (r *InteractionRepository) ListFavoriteTalents(ctx context.Context, userID, page, size int) ([]FavoriteTalent, int64, error) {
	var total int64
	if err := r.db.QueryRowxContext(ctx, `SELECT COUNT(*) FROM talent_favorite f
		JOIN talent_profile tp ON tp.id=f.talent_profile_id
		JOIN `+"`user`"+` u ON u.id=tp.user_id WHERE f.user_id=?`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := `SELECT tp.id,tp.user_id,tp.self_evaluation,tp.skill_summary,tp.project_experience,tp.mbti,tp.status,tp.view_count,
		tp.created_at,tp.updated_at,u.nickname,u.avatar_url,u.school_id,u.major_id,u.grade,u.auth_status,
		f.created_at favorited_at
		FROM talent_favorite f JOIN talent_profile tp ON tp.id=f.talent_profile_id JOIN ` + "`user`" + ` u ON u.id=tp.user_id
		WHERE f.user_id=? ORDER BY f.created_at DESC LIMIT ? OFFSET ?`
	var items []FavoriteTalent
	if err := r.db.SelectContext(ctx, &items, query, userID, size, (page-1)*size); err != nil {
		return nil, 0, err
	}
	if err := r.enrichFavoriteTalentSchoolMajorBatch(ctx, items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *InteractionRepository) enrichFavoriteTalentSchoolMajorBatch(ctx context.Context, items []FavoriteTalent) error {
	schoolIDs := make([]int, 0)
	majorIDs := make([]int, 0)
	for i := range items {
		if items[i].SchoolID != nil {
			schoolIDs = append(schoolIDs, *items[i].SchoolID)
		}
		if items[i].MajorID != nil {
			majorIDs = append(majorIDs, *items[i].MajorID)
		}
	}
	schools := make(map[int]string)
	if len(schoolIDs) > 0 {
		query, args, err := sqlx.In("SELECT id,school_name FROM school WHERE id IN (?)", schoolIDs)
		if err != nil {
			return err
		}
		var rows []struct {
			ID   int    `db:"id"`
			Name string `db:"school_name"`
		}
		if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
			return err
		}
		for _, row := range rows {
			schools[row.ID] = row.Name
		}
	}
	majors := make(map[int]string)
	if len(majorIDs) > 0 {
		query, args, err := sqlx.In("SELECT id,major_name FROM major WHERE id IN (?)", majorIDs)
		if err != nil {
			return err
		}
		var rows []struct {
			ID   int    `db:"id"`
			Name string `db:"major_name"`
		}
		if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(query), args...); err != nil {
			return err
		}
		for _, row := range rows {
			majors[row.ID] = row.Name
		}
	}
	for i := range items {
		if items[i].SchoolID != nil {
			if name, ok := schools[*items[i].SchoolID]; ok {
				items[i].SchoolName = &name
			}
		}
		if items[i].MajorID != nil {
			if name, ok := majors[*items[i].MajorID]; ok {
				items[i].MajorName = &name
			}
		}
	}
	return nil
}

func (r *InteractionRepository) SaveProjectMetadata(ctx context.Context, projectID int, tags *[]string, publisherRole *string, initiatingSchoolID *int) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if publisherRole != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE project SET publisher_role=? WHERE id=?", *publisherRole, projectID); err != nil {
			return err
		}
	}
	if initiatingSchoolID != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE project SET initiating_school_id=? WHERE id=?", *initiatingSchoolID, projectID); err != nil {
			return err
		}
	}
	if tags != nil {
		if _, err := tx.ExecContext(ctx, "DELETE FROM project_tag_relation WHERE project_id=?", projectID); err != nil {
			return err
		}
		for _, name := range *tags {
			res, err := tx.ExecContext(ctx, `INSERT INTO project_tag(name,status) VALUES(?,1)
				ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id),status=1`, name)
			if err != nil {
				return err
			}
			tagID, err := res.LastInsertId()
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO project_tag_relation(project_id,tag_id) VALUES(?,?)", projectID, tagID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
