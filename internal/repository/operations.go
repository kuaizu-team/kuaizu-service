package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// OperationsPerson is the deliberately limited public directory projection.
// Contact details and internal administrator/user identifiers are never selected.
type OperationsPerson struct {
	TalentProfileID *int   `db:"talent_profile_id" json:"talentProfileId,omitempty"`
	Name            string `db:"name" json:"name"`
	SchoolID        int    `db:"school_id" json:"schoolId"`
	SchoolName      string `db:"school_name" json:"schoolName"`
	Position        string `db:"-" json:"position"`
}

type OperationsSchoolTeam struct {
	SchoolID   int                `json:"schoolId"`
	SchoolName string             `json:"schoolName"`
	Leader     OperationsPerson   `json:"leader"`
	Members    []OperationsPerson `json:"members"`
}

type OperationsRepository struct {
	db *sqlx.DB
}

func NewOperationsRepository(db *sqlx.DB) *OperationsRepository {
	return &OperationsRepository{db: db}
}

func (r *OperationsRepository) CountOperatedSchools(ctx context.Context) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count, `
		SELECT COUNT(DISTINCT rel.school_id)
		FROM admin_school_relation rel
		JOIN admin_user au ON au.id = rel.admin_user_id
		WHERE au.role = 2
		  AND au.status = 1
		  AND rel.is_owner = 1
		  AND rel.commission_rate > 0`)
	if err != nil {
		return 0, fmt.Errorf("count operated schools: %w", err)
	}
	return count, nil
}

func (r *OperationsRepository) ListSchoolTeams(ctx context.Context) ([]OperationsSchoolTeam, error) {
	var leaders []OperationsPerson
	err := r.db.SelectContext(ctx, &leaders, `
		SELECT
			tp.id AS talent_profile_id,
			COALESCE(NULLIF(TRIM(au.nickname), ''), NULLIF(TRIM(u.nickname), ''), '快组运营负责人') AS name,
			rel.school_id,
			s.school_name
		FROM admin_school_relation rel
		JOIN admin_user au ON au.id = rel.admin_user_id
		JOIN school s ON s.id = rel.school_id
		LEFT JOIN `+"`user`"+` u
		  ON u.phone = au.phone
		 AND au.phone IS NOT NULL
		 AND au.phone <> ''
		 AND u.user_status = 0
		LEFT JOIN talent_profile tp
		  ON tp.user_id = u.id
		 AND tp.status = 1
		WHERE au.role = 2
		  AND au.status = 1
		  AND rel.is_owner = 1
		  AND rel.commission_rate > 0
		ORDER BY s.school_name ASC, rel.created_at ASC, au.id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list operations leaders: %w", err)
	}

	teams := make([]OperationsSchoolTeam, 0, len(leaders))
	teamIndex := make(map[int]int, len(leaders))
	for _, leader := range leaders {
		leader.Position = "运营负责人"
		teamIndex[leader.SchoolID] = len(teams)
		teams = append(teams, OperationsSchoolTeam{
			SchoolID:   leader.SchoolID,
			SchoolName: leader.SchoolName,
			Leader:     leader,
			Members:    []OperationsPerson{},
		})
	}
	if len(teams) == 0 {
		return teams, nil
	}

	schoolIDs := make([]int, 0, len(teams))
	for _, team := range teams {
		schoolIDs = append(schoolIDs, team.SchoolID)
	}
	query, args, err := sqlx.In(`
		SELECT
			tp.id AS talent_profile_id,
			COALESCE(NULLIF(TRIM(au.nickname), ''), NULLIF(TRIM(u.nickname), ''), '快组运营成员') AS name,
			au.school_id,
			s.school_name
		FROM admin_user au
		JOIN school s ON s.id = au.school_id
		LEFT JOIN `+"`user`"+` u
		  ON u.phone = au.phone
		 AND au.phone IS NOT NULL
		 AND au.phone <> ''
		 AND u.user_status = 0
		LEFT JOIN talent_profile tp
		  ON tp.user_id = u.id
		 AND tp.status = 1
		WHERE au.role = 3
		  AND au.status = 1
		  AND au.school_id IN (?)
		ORDER BY s.school_name ASC, au.created_at ASC, au.id ASC`, schoolIDs)
	if err != nil {
		return nil, fmt.Errorf("build operations members query: %w", err)
	}
	var members []OperationsPerson
	if err := r.db.SelectContext(ctx, &members, r.db.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("list operations members: %w", err)
	}
	for _, member := range members {
		member.Position = "运营成员"
		if index, ok := teamIndex[member.SchoolID]; ok {
			teams[index].Members = append(teams[index].Members, member)
		}
	}
	return teams, nil
}
