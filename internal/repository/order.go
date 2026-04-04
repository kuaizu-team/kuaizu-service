package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// OrderRepository handles order database operations
type OrderRepository struct {
	db *sqlx.DB
}

// NewOrderRepository creates a new OrderRepository
func NewOrderRepository(db *sqlx.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// OrderListParams contains parameters for listing orders
type OrderListParams struct {
	UserID int
	Page   int
	Size   int
	Status *int
}

// ListByUserID retrieves paginated orders for a user
func (r *OrderRepository) ListByUserID(ctx context.Context, params OrderListParams) ([]*models.Order, int64, error) {
	// Build where clause
	where := `WHERE o.user_id = ?`
	args := []interface{}{params.UserID}

	if params.Status != nil {
		where += ` AND o.status = ?`
		args = append(args, *params.Status)
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `order` o %s", where)
	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count orders: %w", err)
	}

	// Query with pagination
	offset := (params.Page - 1) * params.Size
	query := fmt.Sprintf(`
		SELECT
			o.id, o.user_id, o.product_id, o.price, o.quantity, o.actual_paid, o.status,
			o.wx_pay_no, o.pay_time, o.created_at, o.updated_at,
			p.name as product_name
		FROM `+"`order`"+` o
		LEFT JOIN product p ON o.product_id = p.id
		%s
		ORDER BY o.created_at DESC
		LIMIT ? OFFSET ?
	`, where)

	args = append(args, params.Size, offset)

	var orders []*models.Order
	if err := r.db.SelectContext(ctx, &orders, query, args...); err != nil {
		return nil, 0, fmt.Errorf("query orders: %w", err)
	}

	return orders, total, nil
}

// Create creates a new order with items
func (r *OrderRepository) Create(ctx context.Context, order *models.Order) (*models.Order, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert order with product information
	orderQuery := `
		INSERT INTO ` + "`order`" + ` (user_id, product_id, price, quantity, actual_paid, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := tx.ExecContext(ctx, orderQuery,
		order.UserID,
		order.ProductID,
		order.Price,
		order.Quantity,
		order.ActualPaid,
		order.Status)
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get last insert id: %w", err)
	}

	order.ID = int(id)
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return order, nil
}

// GetByID retrieves an order by ID
func (r *OrderRepository) GetByID(ctx context.Context, id int) (*models.Order, error) {
	query := `
		SELECT
			o.id, o.user_id, o.product_id, o.price, o.quantity, o.actual_paid, o.status,
			o.wx_pay_no, o.pay_time, o.created_at, o.updated_at,
			p.name as product_name
		FROM ` + "`order`" + ` o
		LEFT JOIN product p ON o.product_id = p.id
		WHERE o.id = ?
	`

	var o models.Order
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&o); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get order by id: %w", err)
	}

	return &o, nil
}

// UpdatePaymentStatus updates order payment status
func (r *OrderRepository) UpdatePaymentStatus(ctx context.Context, id int, status int, wxPayNo string, payTime time.Time) error {
	query := `
		UPDATE ` + "`order`" + ` SET
			status = ?,
			wx_pay_no = ?,
			pay_time = ?,
			updated_at = NOW()
		WHERE id = ?
	`

	_, err := r.db.ExecContext(ctx, query, status, wxPayNo, payTime, id)
	if err != nil {
		return fmt.Errorf("update payment status: %w", err)
	}

	return nil
}

// UpdatePaymentStatusTx updates order payment status within a transaction
func (r *OrderRepository) UpdatePaymentStatusTx(ctx context.Context, tx *sqlx.Tx, id int, status int, wxPayNo string, payTime time.Time) error {
	query := `
		UPDATE ` + "`order`" + ` SET
			status = ?,
			wx_pay_no = ?,
			pay_time = ?,
			updated_at = NOW()
		WHERE id = ?
	`

	_, err := tx.ExecContext(ctx, query, status, wxPayNo, payTime, id)
	if err != nil {
		return fmt.Errorf("update payment status: %w", err)
	}

	return nil
}

// UpdateStatus updates only the order status
func (r *OrderRepository) UpdateStatus(ctx context.Context, id int, status int) error {
	query := `
		UPDATE ` + "`order`" + ` SET
			status = ?,
			updated_at = NOW()
		WHERE id = ?
	`

	_, err := r.db.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("update order status: %w", err)
	}

	return nil
}
