package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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

// AdminOrderListParams contains parameters for admin order list
type AdminOrderListParams struct {
	Page       int
	Size       int
	OrderNo    *string // 订单号模糊搜索（匹配 CAST(o.id AS CHAR)，因 out_trade_no 列未写入 DB）
	WxPayNo    *string // wx_pay_no 模糊搜索
	Nickname   *string // 用户昵称模糊搜索（JOIN user 表）
	SchoolName *string // 学校名称模糊搜索（JOIN school 表）
	SchoolID   *int    // admin 权限过滤：只返回该学校用户的订单
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
			o.refund_status, o.refund_reason, o.refund_apply_time,
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
			o.refund_status, o.refund_reason, o.refund_apply_time,
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

// UpdateRefundApply records a refund application for a paid order that has not applied before.
func (r *OrderRepository) UpdateRefundApply(ctx context.Context, id int, reason string) (bool, error) {
	query := `
		UPDATE ` + "`order`" + ` SET
			refund_status = ?,
			refund_reason = ?,
			refund_apply_time = NOW(),
			updated_at = NOW()
		WHERE id = ?
			AND status = ?
			AND refund_status = ?
	`

	result, err := r.db.ExecContext(ctx, query, 1, reason, id, models.OrderStatusPaid, 0)
	if err != nil {
		return false, fmt.Errorf("update refund apply: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read refund apply affected rows: %w", err)
	}

	return affected > 0, nil
}

// AdminList retrieves paginated orders for admin with multi-condition fuzzy search.
// JOINs user and school to support nickname/schoolName filters.
func (r *OrderRepository) AdminList(ctx context.Context, params AdminOrderListParams) ([]*models.Order, int64, error) {
	conditions := []string{"1=1"}
	args := []interface{}{}

	if params.OrderNo != nil && *params.OrderNo != "" {
		// out_trade_no 列从未写入 DB；以订单 ID 字符串作为订单号匹配
		conditions = append(conditions, "CAST(o.id AS CHAR) LIKE ?")
		args = append(args, "%"+*params.OrderNo+"%")
	}
	if params.WxPayNo != nil && *params.WxPayNo != "" {
		conditions = append(conditions, "o.wx_pay_no LIKE ?")
		args = append(args, "%"+*params.WxPayNo+"%")
	}
	if params.Nickname != nil && *params.Nickname != "" {
		conditions = append(conditions, "u.nickname LIKE ?")
		args = append(args, "%"+*params.Nickname+"%")
	}
	if params.SchoolName != nil && *params.SchoolName != "" {
		conditions = append(conditions, "s.school_name LIKE ?")
		args = append(args, "%"+*params.SchoolName+"%")
	}
	if params.SchoolID != nil {
		conditions = append(conditions, "u.school_id = ?")
		args = append(args, *params.SchoolID)
	}

	whereClause := strings.Join(conditions, " AND ")

	// Base FROM + JOIN (same for count and data queries)
	fromJoin := "`order` o " +
		"LEFT JOIN `user` u ON o.user_id = u.id " +
		"LEFT JOIN school s ON u.school_id = s.id " +
		"LEFT JOIN product p ON o.product_id = p.id"

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", fromJoin, whereClause)
	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count admin orders: %w", err)
	}

	offset := (params.Page - 1) * params.Size
	query := fmt.Sprintf(`
		SELECT
			o.id, o.user_id, o.product_id, o.price, o.quantity, o.actual_paid, o.status,
			o.refund_status, o.refund_reason, o.refund_apply_time,
			o.wx_pay_no, o.pay_time, o.created_at, o.updated_at,
			p.name  AS product_name,
			u.nickname AS user_nickname,
			s.school_name
		FROM %s
		WHERE %s
		ORDER BY o.created_at DESC
		LIMIT ? OFFSET ?
	`, fromJoin, whereClause)

	dataArgs := make([]interface{}, 0, len(args)+2)
	dataArgs = append(dataArgs, args...)
	dataArgs = append(dataArgs, params.Size, offset)

	var orders []*models.Order
	if err := r.db.SelectContext(ctx, &orders, query, dataArgs...); err != nil {
		return nil, 0, fmt.Errorf("query admin orders: %w", err)
	}

	return orders, total, nil
}

// AdminGetByID retrieves a single order with full user, school, and product info for admin.
func (r *OrderRepository) AdminGetByID(ctx context.Context, id int) (*models.Order, error) {
	query := `
		SELECT
			o.id, o.user_id, o.product_id, o.price, o.quantity, o.actual_paid, o.status,
			o.refund_status, o.refund_reason, o.refund_apply_time,
			o.wx_pay_no, o.pay_time, o.created_at, o.updated_at,
			p.name        AS product_name,
			p.description AS product_description,
			u.nickname    AS user_nickname,
			u.school_id   AS user_school_id,
			s.school_name
		FROM ` + "`order`" + ` o
		LEFT JOIN ` + "`user`" + ` u ON o.user_id = u.id
		LEFT JOIN school s ON u.school_id = s.id
		LEFT JOIN product p ON o.product_id = p.id
		WHERE o.id = ?
	`

	var o models.Order
	if err := r.db.QueryRowxContext(ctx, query, id).StructScan(&o); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("admin get order by id: %w", err)
	}

	return &o, nil
}
