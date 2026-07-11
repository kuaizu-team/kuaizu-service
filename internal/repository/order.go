package repository

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kuaizu-team/kuaizu-service/internal/models"
)

// OrderRepository handles order database operations.
type OrderRepository struct {
	db *sqlx.DB
}

// NewOrderRepository creates a new OrderRepository.
func NewOrderRepository(db *sqlx.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// OrderListParams contains parameters for listing orders.
type OrderListParams struct {
	UserID       int
	Page         int
	Size         int
	Status       *int
	RefundStatus *int
	AfterSale    bool
}

// AdminOrderListParams contains parameters for admin order list.
type AdminOrderListParams struct {
	Page                int
	Size                int
	OrderNo             *string
	WxPayNo             *string
	Nickname            *string
	SchoolName          *string
	SchoolID            *int
	UserID              *int
	SettlementStatus    *int
	RefundStatus        *int
	RefundApplicantType *int
}

type RevenueStats struct {
	TotalRevenue                   int64 `json:"totalRevenue"`
	WeekRevenue                    int64 `json:"weekRevenue"`
	PendingSettlementAmount        int64 `json:"pendingSettlementAmount"`
	PendingConsumerRefundCount     int64 `json:"pendingConsumerRefundCount"`
	PendingSchoolAdminRefundCount  int64 `json:"pendingSchoolAdminRefundCount"`
	PendingConsumerRefundAmount    int64 `json:"pendingConsumerRefundAmount"`
	PendingSchoolAdminRefundAmount int64 `json:"pendingSchoolAdminRefundAmount"`
	SchoolID                       *int  `json:"schoolId"`
}

type SettlementResult struct {
	BatchNo     string `json:"batchNo"`
	OrderCount  int    `json:"orderCount"`
	TotalAmount int64  `json:"totalAmount"`
}

const orderFinanceCols = `
	o.settlement_status, o.refund_status, o.refund_reason, o.refund_apply_time,
	o.refund_applicant_type, o.refund_applicant_admin_id,
	o.reject_reason, o.reject_time, o.refund_withdraw_time,
	o.refund_handle_time, o.refund_operator_admin_id,
	o.settlement_batch_no, o.settlement_time, o.settlement_operator_admin_id`

// ListByUserID retrieves paginated orders for a user.
func (r *OrderRepository) ListByUserID(ctx context.Context, params OrderListParams) ([]*models.Order, int64, error) {
	where := `WHERE o.user_id = ?`
	args := []interface{}{params.UserID}

	if params.AfterSale {
		where += ` AND (o.status = ? OR o.refund_status IN (?, ?, ?))`
		args = append(args, models.OrderStatusRefunded, 1, 2, 3)
	} else {
		if params.Status != nil {
			where += ` AND o.status = ?`
			args = append(args, *params.Status)
		}
		if params.RefundStatus != nil {
			where += ` AND o.refund_status = ?`
			args = append(args, *params.RefundStatus)
		}
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `order` o %s", where)
	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count orders: %w", err)
	}

	offset := (params.Page - 1) * params.Size
	query := fmt.Sprintf(`
		SELECT
			o.id, o.user_id, o.product_id, o.template_code, o.template_name, o.price, o.quantity, o.actual_paid, o.status,
			%s,
			o.wx_pay_no, o.pay_time, o.created_at, o.updated_at,
			p.name as product_name
		FROM `+"`order`"+` o
		LEFT JOIN product p ON o.product_id = p.id
		%s
		ORDER BY o.created_at DESC
		LIMIT ? OFFSET ?
	`, orderFinanceCols, where)

	args = append(args, params.Size, offset)

	var orders []*models.Order
	if err := r.db.SelectContext(ctx, &orders, query, args...); err != nil {
		return nil, 0, fmt.Errorf("query orders: %w", err)
	}

	return orders, total, nil
}

// Create creates a new order with items.
func (r *OrderRepository) Create(ctx context.Context, order *models.Order) (*models.Order, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	orderQuery := `
		INSERT INTO ` + "`order`" + ` (user_id, product_id, price, quantity, actual_paid, status, settlement_status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
	`

	result, err := tx.ExecContext(ctx, orderQuery,
		order.UserID,
		order.ProductID,
		order.Price,
		order.Quantity,
		order.ActualPaid,
		order.Status,
		2)
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get last insert id: %w", err)
	}

	order.ID = int(id)
	order.SettlementStatus = 2
	order.CreatedAt = time.Now()
	order.UpdatedAt = time.Now()

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return order, nil
}

// GetByID retrieves an order by ID.
func (r *OrderRepository) GetByID(ctx context.Context, id int) (*models.Order, error) {
	query := `
		SELECT
			o.id, o.user_id, o.product_id, o.template_code, o.template_name, o.price, o.quantity, o.actual_paid, o.status,
			` + orderFinanceCols + `,
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

// UpdatePaymentStatus updates order payment status.
func (r *OrderRepository) UpdatePaymentStatus(ctx context.Context, id int, status int, wxPayNo string, payTime time.Time) error {
	query := `
		UPDATE ` + "`order`" + ` SET
			status = ?,
			settlement_status = CASE
				WHEN ? = 1 AND refund_status = 0 THEN 0
				ELSE settlement_status
			END,
			wx_pay_no = ?,
			pay_time = ?,
			updated_at = NOW()
		WHERE id = ?
	`

	if _, err := r.db.ExecContext(ctx, query, status, status, wxPayNo, payTime, id); err != nil {
		return fmt.Errorf("update payment status: %w", err)
	}

	return nil
}

// UpdatePaymentStatusTx updates order payment status within a transaction.
func (r *OrderRepository) UpdatePaymentStatusTx(ctx context.Context, tx *sqlx.Tx, id int, status int, wxPayNo string, payTime time.Time) error {
	query := `
		UPDATE ` + "`order`" + ` SET
			status = ?,
			settlement_status = CASE
				WHEN ? = 1 AND refund_status = 0 THEN 0
				ELSE settlement_status
			END,
			wx_pay_no = ?,
			pay_time = ?,
			updated_at = NOW()
		WHERE id = ?
	`

	if _, err := tx.ExecContext(ctx, query, status, status, wxPayNo, payTime, id); err != nil {
		return fmt.Errorf("update payment status: %w", err)
	}

	return nil
}

// UpdateStatus updates only the order status.
func (r *OrderRepository) UpdateStatus(ctx context.Context, id int, status int) error {
	query := `
		UPDATE ` + "`order`" + ` SET
			status = ?,
			updated_at = NOW()
		WHERE id = ?
	`

	if _, err := r.db.ExecContext(ctx, query, status, id); err != nil {
		return fmt.Errorf("update order status: %w", err)
	}

	return nil
}

// UpdateRefundApply records a refund application for a paid order that has not applied before.
func (r *OrderRepository) UpdateRefundApply(ctx context.Context, id int, reason string, applicantType int, applicantAdminID *int) (bool, error) {
	query := `
		UPDATE ` + "`order`" + ` SET
			refund_status = ?,
			refund_reason = ?,
			refund_apply_time = NOW(),
			refund_applicant_type = ?,
			refund_applicant_admin_id = ?,
			reject_reason = NULL,
			reject_time = NULL,
			refund_withdraw_time = NULL,
			settlement_status = CASE
				WHEN settlement_status = 0 THEN 3
				ELSE settlement_status
			END,
			updated_at = NOW()
		WHERE id = ?
			AND status = ?
			AND refund_status = ?
	`

	result, err := r.db.ExecContext(ctx, query, 1, reason, applicantType, applicantAdminID, id, models.OrderStatusPaid, 0)
	if err != nil {
		return false, fmt.Errorf("update refund apply: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read refund apply affected rows: %w", err)
	}

	return affected > 0, nil
}

// AdminRejectRefund marks a pending refund as rejected.
func (r *OrderRepository) AdminRejectRefund(ctx context.Context, id int, reason string, adminID int) (bool, error) {
	query := `
		UPDATE ` + "`order`" + ` SET
			refund_status = 3,
			reject_reason = ?,
			reject_time = NOW(),
			refund_operator_admin_id = ?,
			settlement_status = CASE
				WHEN settlement_status = 3 THEN 0
				ELSE settlement_status
			END,
			updated_at = NOW()
		WHERE id = ?
			AND refund_status = 1
	`
	result, err := r.db.ExecContext(ctx, query, reason, adminID, id)
	if err != nil {
		return false, fmt.Errorf("reject refund: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read reject refund affected rows: %w", err)
	}
	return affected > 0, nil
}

// WithdrawRefund marks a pending refund as withdrawn.
func (r *OrderRepository) WithdrawRefund(ctx context.Context, id int) (bool, error) {
	query := `
		UPDATE ` + "`order`" + ` SET
			refund_status = 4,
			refund_withdraw_time = NOW(),
			settlement_status = CASE
				WHEN settlement_status = 3 THEN 0
				ELSE settlement_status
			END,
			updated_at = NOW()
		WHERE id = ?
			AND refund_status = 1
	`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return false, fmt.Errorf("withdraw refund: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read withdraw refund affected rows: %w", err)
	}
	return affected > 0, nil
}

// AdminReviewRefund marks a pending refund as successful.
func (r *OrderRepository) AdminReviewRefund(ctx context.Context, id int, adminID int) (bool, error) {
	query := `
		UPDATE ` + "`order`" + ` SET
			refund_status = 2,
			status = ?,
			refund_handle_time = NOW(),
			refund_operator_admin_id = ?,
			settlement_status = CASE
				WHEN settlement_status = 3 THEN 2
				ELSE settlement_status
			END,
			updated_at = NOW()
		WHERE id = ?
			AND refund_status = 1
	`
	result, err := r.db.ExecContext(ctx, query, models.OrderStatusRefunded, adminID, id)
	if err != nil {
		return false, fmt.Errorf("review refund: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read review refund affected rows: %w", err)
	}
	return affected > 0, nil
}

// AdminList retrieves paginated orders for admin with multi-condition fuzzy search.
func (r *OrderRepository) AdminList(ctx context.Context, params AdminOrderListParams) ([]*models.Order, int64, error) {
	conditions := []string{"1=1"}
	args := []interface{}{}

	if params.OrderNo != nil && *params.OrderNo != "" {
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
	if params.UserID != nil {
		conditions = append(conditions, "o.user_id = ?")
		args = append(args, *params.UserID)
	}
	if params.SettlementStatus != nil {
		conditions = append(conditions, "o.settlement_status = ?")
		args = append(args, *params.SettlementStatus)
		if *params.SettlementStatus == 0 {
			conditions = append(conditions, "o.status = 1", "o.refund_status = 0")
		}
	}
	if params.RefundStatus != nil {
		conditions = append(conditions, "o.refund_status = ?")
		args = append(args, *params.RefundStatus)
	}
	if params.RefundApplicantType != nil {
		conditions = append(conditions, "o.refund_applicant_type = ?")
		args = append(args, *params.RefundApplicantType)
	}

	whereClause := strings.Join(conditions, " AND ")
	fromJoin := "`order` o " +
		"LEFT JOIN `user` u ON o.user_id = u.id " +
		"LEFT JOIN school s ON u.school_id = s.id " +
		"LEFT JOIN product p ON o.product_id = p.id"

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", fromJoin, whereClause)
	var total int64
	if err := r.db.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count admin orders: %w", err)
	}

	offset := (params.Page - 1) * params.Size
	query := fmt.Sprintf(`
		SELECT
			o.id, o.user_id, o.product_id, o.template_code, o.template_name, o.price, o.quantity, o.actual_paid, o.status,
			%s,
			o.wx_pay_no, o.pay_time, o.created_at, o.updated_at,
			p.name AS product_name,
			u.nickname AS user_nickname,
			u.school_id AS user_school_id,
			s.school_name
		FROM %s
		WHERE %s
		ORDER BY o.created_at DESC
		LIMIT ? OFFSET ?
	`, orderFinanceCols, fromJoin, whereClause)

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
			o.id, o.user_id, o.product_id, o.template_code, o.template_name, o.price, o.quantity, o.actual_paid, o.status,
			` + orderFinanceCols + `,
			o.wx_pay_no, o.pay_time, o.created_at, o.updated_at,
			p.name AS product_name,
			p.description AS product_description,
			u.nickname AS user_nickname,
			u.school_id AS user_school_id,
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

func (r *OrderRepository) RevenueStats(ctx context.Context, schoolID *int) (*RevenueStats, error) {
	stats := &RevenueStats{SchoolID: schoolID}
	from := "`order` o LEFT JOIN `user` u ON o.user_id = u.id"
	schoolWhere := ""
	args := []interface{}{}
	if schoolID != nil {
		schoolWhere = " AND u.school_id = ?"
		args = append(args, *schoolID)
	}

	queries := []struct {
		sql string
		dst *int64
	}{
		{fmt.Sprintf("SELECT CAST(COALESCE(ROUND(SUM(o.actual_paid) * 100), 0) AS SIGNED) FROM %s WHERE o.status = 1 AND o.refund_status != 2%s", from, schoolWhere), &stats.TotalRevenue},
		{fmt.Sprintf("SELECT CAST(COALESCE(ROUND(SUM(o.actual_paid) * 100), 0) AS SIGNED) FROM %s WHERE o.status = 1 AND o.refund_status != 2 AND o.pay_time >= DATE_SUB(NOW(), INTERVAL 7 DAY)%s", from, schoolWhere), &stats.WeekRevenue},
		{fmt.Sprintf("SELECT CAST(COALESCE(ROUND(SUM(o.actual_paid) * 100), 0) AS SIGNED) FROM %s WHERE o.status = 1 AND o.settlement_status = 0 AND o.refund_status = 0%s", from, schoolWhere), &stats.PendingSettlementAmount},
		{fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE o.refund_status = 1 AND o.refund_applicant_type = 0%s", from, schoolWhere), &stats.PendingConsumerRefundCount},
		{fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE o.refund_status = 1 AND o.refund_applicant_type = 1%s", from, schoolWhere), &stats.PendingSchoolAdminRefundCount},
		{fmt.Sprintf("SELECT CAST(COALESCE(ROUND(SUM(o.actual_paid) * 100), 0) AS SIGNED) FROM %s WHERE o.refund_status = 1 AND o.refund_applicant_type = 0%s", from, schoolWhere), &stats.PendingConsumerRefundAmount},
		{fmt.Sprintf("SELECT CAST(COALESCE(ROUND(SUM(o.actual_paid) * 100), 0) AS SIGNED) FROM %s WHERE o.refund_status = 1 AND o.refund_applicant_type = 1%s", from, schoolWhere), &stats.PendingSchoolAdminRefundAmount},
	}

	for _, q := range queries {
		if err := r.db.QueryRowxContext(ctx, q.sql, args...).Scan(q.dst); err != nil {
			return nil, fmt.Errorf("query revenue stats: %w", err)
		}
	}
	return stats, nil
}

type settlementOrderRow struct {
	ID         int     `db:"id"`
	ActualPaid float64 `db:"actual_paid"`
}

func (r *OrderRepository) SettleSchoolPendingOrders(ctx context.Context, schoolID int, adminID int, commissionRate float64, remark *string) (*SettlementResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin settle transaction: %w", err)
	}
	defer tx.Rollback()

	var rows []settlementOrderRow
	if err := tx.SelectContext(ctx, &rows, `
		SELECT o.id, o.actual_paid
		FROM `+"`order`"+` o
		JOIN `+"`user`"+` u ON o.user_id = u.id
		WHERE u.school_id = ?
			AND o.status = 1
			AND o.settlement_status = 0
			AND o.refund_status = 0
		FOR UPDATE
	`, schoolID); err != nil {
		return nil, fmt.Errorf("select settle orders: %w", err)
	}
	if len(rows) == 0 {
		return &SettlementResult{BatchNo: "", OrderCount: 0, TotalAmount: 0}, nil
	}

	orderAmounts, totalAmount := calculateCommissionAmounts(rows, commissionRate)

	batchNo := fmt.Sprintf("SETTLE-%s-%d", time.Now().Format("20060102150405"), schoolID)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO settlement_record (batch_no, school_id, order_count, total_amount, operator_admin_id, remark, created_at)
		VALUES (?, ?, ?, ?, ?, ?, NOW())
	`, batchNo, schoolID, len(rows), totalAmount, adminID, remark)
	if err != nil {
		return nil, fmt.Errorf("insert settlement record: %w", err)
	}
	recordID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get settlement record id: %w", err)
	}

	for i, row := range rows {
		amount := orderAmounts[i]
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO settlement_record_order (settlement_record_id, order_id, amount, created_at)
			VALUES (?, ?, ?, NOW())
		`, recordID, row.ID, amount); err != nil {
			return nil, fmt.Errorf("insert settlement record order: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE `+"`order`"+` SET
				settlement_status = 1,
				settlement_batch_no = ?,
				settlement_time = NOW(),
				settlement_operator_admin_id = ?,
				updated_at = NOW()
			WHERE id = ?
		`, batchNo, adminID, row.ID); err != nil {
			return nil, fmt.Errorf("update settled order: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit settle transaction: %w", err)
	}
	return &SettlementResult{BatchNo: batchNo, OrderCount: len(rows), TotalAmount: totalAmount}, nil
}

func calculateCommissionAmounts(rows []settlementOrderRow, commissionRate float64) ([]int64, int64) {
	amounts := make([]int64, len(rows))
	fullAmount := int64(0)
	allocatedAmount := int64(0)
	for i, row := range rows {
		paidFen := int64(math.Round(row.ActualPaid * 100))
		fullAmount += paidFen
		amounts[i] = int64(math.Round(float64(paidFen) * commissionRate / 100))
		allocatedAmount += amounts[i]
	}
	totalAmount := int64(math.Round(float64(fullAmount) * commissionRate / 100))
	if len(amounts) > 0 {
		amounts[len(amounts)-1] += totalAmount - allocatedAmount
	}
	return amounts, totalAmount
}
