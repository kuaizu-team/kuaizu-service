package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/trv3wood/kuaizu-server/internal/models"
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
	err := r.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count orders: %w", err)
	}

	// Query with pagination
	offset := (params.Page - 1) * params.Size
	query := fmt.Sprintf(`
		SELECT
			o.id, o.user_id, o.actual_paid, o.status,
			o.wx_pay_no, o.pay_time, o.created_at, o.updated_at
		FROM `+"`order`"+` o
		%s
		ORDER BY o.created_at DESC
		LIMIT ? OFFSET ?
	`, where)

	args = append(args, params.Size, offset)

	rows, err := r.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query orders: %w", err)
	}
	defer rows.Close()

	var orders []*models.Order
	for rows.Next() {
		var o models.Order
		err := rows.Scan(
			&o.ID, &o.UserID, &o.ActualPaid, &o.Status,
			&o.WxPayNo, &o.PayTime, &o.CreatedAt, &o.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan order: %w", err)
		}
		orders = append(orders, &o)
	}

	// Load order items for each order
	for _, order := range orders {
		items, err := r.GetOrderItems(ctx, order.ID)
		if err != nil {
			return nil, 0, fmt.Errorf("get order items: %w", err)
		}
		order.Items = items
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

	// Insert order
	orderQuery := `
		INSERT INTO ` + "`order`" + ` (user_id, actual_paid, status, created_at, updated_at)
		VALUES (?, ?, ?, NOW(), NOW())
	`

	result, err := tx.ExecContext(ctx, orderQuery, order.UserID, order.ActualPaid, order.Status)
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

	// Insert order items
	if len(order.Items) > 0 {
		itemQuery := `
			INSERT INTO order_item (order_id, product_id, price, quantity)
			VALUES (?, ?, ?, ?)
		`
		for _, item := range order.Items {
			_, err := tx.ExecContext(ctx, itemQuery, order.ID, item.ProductID, item.Price, item.Quantity)
			if err != nil {
				return nil, fmt.Errorf("create order item: %w", err)
			}
			item.OrderID = order.ID
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return order, nil
}

// GetByID retrieves an order by ID
func (r *OrderRepository) GetByID(ctx context.Context, id int) (*models.Order, error) {
	query := `
		SELECT
			o.id, o.user_id, o.actual_paid, o.status,
			o.wx_pay_no, o.pay_time, o.created_at, o.updated_at
		FROM ` + "`order`" + ` o
		WHERE o.id = ?
	`

	var o models.Order
	err := r.db.QueryRowxContext(ctx, query, id).Scan(
		&o.ID, &o.UserID, &o.ActualPaid, &o.Status,
		&o.WxPayNo, &o.PayTime, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("get order by id: %w", err)
	}

	// Load order items
	items, err := r.GetOrderItems(ctx, o.ID)
	if err != nil {
		return nil, fmt.Errorf("get order items: %w", err)
	}
	o.Items = items

	return &o, nil
}

// GetOrderItems retrieves order items for an order
func (r *OrderRepository) GetOrderItems(ctx context.Context, orderID int) ([]*models.OrderItem, error) {
	query := `
		SELECT
			oi.id, oi.order_id, oi.product_id, oi.price, oi.quantity,
			p.name as product_name
		FROM order_item oi
		LEFT JOIN product p ON oi.product_id = p.id
		WHERE oi.order_id = ?
	`

	rows, err := r.db.QueryxContext(ctx, query, orderID)
	if err != nil {
		return nil, fmt.Errorf("query order items: %w", err)
	}
	defer rows.Close()

	var items []*models.OrderItem
	for rows.Next() {
		var item models.OrderItem
		err := rows.Scan(
			&item.ID, &item.OrderID, &item.ProductID, &item.Price, &item.Quantity,
			&item.ProductName,
		)
		if err != nil {
			return nil, fmt.Errorf("scan order item: %w", err)
		}
		items = append(items, &item)
	}

	return items, nil
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
