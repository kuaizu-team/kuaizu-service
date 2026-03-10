package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/trv3wood/kuaizu-server/internal/models"
)

// OliveBranchRepository handles olive branch database operations
type OliveBranchRepository struct {
	db *sqlx.DB
}

// NewOliveBranchRepository creates a new OliveBranchRepository
func NewOliveBranchRepository(db *sqlx.DB) *OliveBranchRepository {
	return &OliveBranchRepository{db: db}
}

// OliveBranchListParams contains parameters for listing olive branches
type OliveBranchListParams struct {
	SenderID   int
	ReceiverID int
	Page       int
	Size       int
	Status     *int
}

// ListByReceiverID retrieves paginated olive branches received by a user
func (r *OliveBranchRepository) ListByReceiverID(ctx context.Context, params OliveBranchListParams) ([]models.OliveBranch, int64, error) {
	// Count total
	countArgs := []interface{}{params.ReceiverID}
	countQuery := `SELECT COUNT(*) FROM olive_branch_record WHERE receiver_id = ?`
	if params.Status != nil {
		countQuery += ` AND status = ?`
		countArgs = append(countArgs, *params.Status)
	}

	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count olive branches: %w", err)
	}

	// Query with pagination
	offset := (params.Page - 1) * params.Size
	args := []interface{}{params.ReceiverID}

	query := `
		SELECT 
			ob.id, ob.sender_id, ob.receiver_id, ob.related_project_id,
			ob.type, ob.cost_type, ob.status,
			ob.created_at, ob.updated_at,
			p.name AS project_name,
			s.id, s.nickname, s.phone, s.email, s.auth_status,
			s.avatar_url
		FROM olive_branch_record ob
		LEFT JOIN project p ON ob.related_project_id = p.id
		LEFT JOIN ` + "`user`" + ` s ON ob.sender_id = s.id
		WHERE ob.receiver_id = ?
	`
	if params.Status != nil {
		query += ` AND ob.status = ?`
		args = append(args, *params.Status)
	}
	query += ` ORDER BY ob.created_at DESC LIMIT ? OFFSET ?`
	args = append(args, params.Size, offset)

	rows, err := r.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query olive branches: %w", err)
	}
	defer rows.Close()

	var records []models.OliveBranch
	for rows.Next() {
		var ob models.OliveBranch
		var sender models.User

		err := rows.Scan(
			&ob.ID, &ob.SenderID, &ob.ReceiverID, &ob.RelatedProjectID,
			&ob.Type, &ob.CostType, &ob.Status,
			&ob.CreatedAt, &ob.UpdatedAt,
			&ob.ProjectName,
			&sender.ID, &sender.Nickname, &sender.Phone, &sender.Email, &sender.AuthStatus,
			&sender.AvatarUrl,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan olive branch: %w", err)
		}
		ob.Sender = &sender
		records = append(records, ob)
	}

	return records, total, nil
}

// GetByID retrieves an olive branch by ID
func (r *OliveBranchRepository) GetByID(ctx context.Context, id int) (*models.OliveBranch, error) {
	query := `
		SELECT 
			ob.id, ob.sender_id, ob.receiver_id, ob.related_project_id,
			ob.type, ob.cost_type, ob.status,
			ob.created_at, ob.updated_at,
			p.name AS project_name
		FROM olive_branch_record ob
		LEFT JOIN project p ON ob.related_project_id = p.id
		WHERE ob.id = ?
	`

	var ob models.OliveBranch
	err := r.db.QueryRowxContext(ctx, query, id).Scan(
		&ob.ID, &ob.SenderID, &ob.ReceiverID, &ob.RelatedProjectID,
		&ob.Type, &ob.CostType, &ob.Status,
		&ob.CreatedAt, &ob.UpdatedAt,
		&ob.ProjectName,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query olive branch by id: %w", err)
	}

	return &ob, nil
}

// Create creates a new olive branch record
func (r *OliveBranchRepository) Create(ctx context.Context, ob *models.OliveBranch) error {
	query := `
		INSERT INTO olive_branch_record (
			sender_id, receiver_id, related_project_id,
			type, cost_type, status
		) VALUES (?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.ExecContext(ctx, query,
		ob.SenderID, ob.ReceiverID, ob.RelatedProjectID,
		ob.Type, ob.CostType, ob.Status,
	)
	if err != nil {
		return fmt.Errorf("create olive branch: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	ob.ID = int(id)

	return nil
}

// ExistsPending checks if there is a pending (status=0) olive branch from sender to receiver.
func (r *OliveBranchRepository) ExistsPending(ctx context.Context, senderID, receiverID, relatedProjectID int) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM olive_branch_record WHERE sender_id = ? AND receiver_id = ? AND related_project_id = ? AND status = 0`
	if err := r.db.QueryRowxContext(ctx, query, senderID, receiverID, relatedProjectID).Scan(&count); err != nil {
		return false, fmt.Errorf("check pending olive branch: %w", err)
	}
	return count > 0, nil
}

// UpdateStatus updates the status of an olive branch
func (r *OliveBranchRepository) UpdateStatus(ctx context.Context, id int, status int) error {
	query := `UPDATE olive_branch_record SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("update olive branch status: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("olive branch not found")
	}

	return nil
}

// ListBySenderID retrieves paginated olive branches sent by a user
func (r *OliveBranchRepository) ListBySenderID(ctx context.Context, params OliveBranchListParams) ([]models.OliveBranch, int64, error) {
	// Count total
	countArgs := []interface{}{params.SenderID}
	countQuery := `SELECT COUNT(*) FROM olive_branch_record WHERE sender_id = ?`
	if params.Status != nil {
		countQuery += ` AND status = ?`
		countArgs = append(countArgs, *params.Status)
	}

	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count olive branches: %w", err)
	}

	// Query with pagination
	offset := (params.Page - 1) * params.Size
	args := []interface{}{params.SenderID}

	query := `
		SELECT 
			ob.id, ob.sender_id, ob.receiver_id, ob.related_project_id,
			ob.type, ob.cost_type, ob.status,
			ob.created_at, ob.updated_at,
			p.name AS project_name,
			r.id, r.nickname, r.phone, r.email, r.auth_status,
			r.avatar_url
		FROM olive_branch_record ob
		LEFT JOIN project p ON ob.related_project_id = p.id
		LEFT JOIN ` + "`user`" + ` r ON ob.receiver_id = r.id
		WHERE ob.sender_id = ?
	`
	if params.Status != nil {
		query += ` AND ob.status = ?`
		args = append(args, *params.Status)
	}
	query += ` ORDER BY ob.created_at DESC LIMIT ? OFFSET ?`
	args = append(args, params.Size, offset)

	rows, err := r.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query olive branches: %w", err)
	}
	defer rows.Close()

	var records []models.OliveBranch
	for rows.Next() {
		var ob models.OliveBranch
		var receiver models.User

		err := rows.Scan(
			&ob.ID, &ob.SenderID, &ob.ReceiverID, &ob.RelatedProjectID,
			&ob.Type, &ob.CostType, &ob.Status,
			&ob.CreatedAt, &ob.UpdatedAt,
			&ob.ProjectName,
			&receiver.ID, &receiver.Nickname, &receiver.Phone, &receiver.Email, &receiver.AuthStatus,
			&receiver.AvatarUrl,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan olive branch: %w", err)
		}
		ob.Receiver = &receiver
		records = append(records, ob)
	}

	return records, total, nil
}
