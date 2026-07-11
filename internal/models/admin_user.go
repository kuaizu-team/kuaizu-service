package models

import "time"

// AdminUser represents an admin user in the database
type AdminUser struct {
	ID             int       `db:"id"`
	Username       string    `db:"username"`
	PasswordHash   string    `db:"password_hash"`
	Nickname       *string   `db:"nickname"`
	Role           int       `db:"role"`      // 1=超级管理员 2=校区超级管理员 3=校区管理员
	SchoolID       *int      `db:"school_id"` // 校区角色必填，超级管理员为NULL
	Status         int       `db:"status"`    // 1=enabled, 0=disabled
	FinanceRemark  *string   `db:"finance_remark"`
	CommissionRate float64   `db:"commission_rate"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`

	// Joined field — populated when querying with LEFT JOIN school
	SchoolName *string `db:"school_name"`
}
