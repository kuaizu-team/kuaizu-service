package models

import "time"

// AdminUser represents an admin user in the database
type AdminUser struct {
	ID                int        `db:"id"`
	Username          string     `db:"username"`
	PasswordHash      string     `db:"password_hash"`
	PasswordEncrypted *string    `db:"password_encrypted"`
	Nickname          *string    `db:"nickname"`
	Phone             *string    `db:"phone"`
	Role              int        `db:"role"`      // 1=超级管理员 2=校区超级管理员 3=校区管理员 4=赛事管理员
	SchoolID          *int       `db:"school_id"` // 校区角色必填，超级管理员为NULL
	Status            int        `db:"status"`    // 1=enabled, 0=disabled
	FinanceRemark     *string    `db:"finance_remark"`
	CommissionRate    float64    `db:"commission_rate"`
	JoinDate          *time.Time `db:"join_date"`
	Intro             *string    `db:"intro"`
	ArticleURL        *string    `db:"article_url"`
	CreatedAt         time.Time  `db:"created_at"`
	UpdatedAt         time.Time  `db:"updated_at"`

	// Joined field — populated when querying with LEFT JOIN school
	SchoolName *string `db:"school_name"`

	// Schools is authoritative for school super admins. SchoolID/SchoolName and
	// CommissionRate remain for the single-school roles and legacy clients.
	Schools []AdminSchoolRelation `db:"-"`
}

// AdminSchoolRelation separates unique operational ownership from school access.
// IsOwner identifies the one delegation owner; every positive commission rate
// grants data and operational access to that school.
type AdminSchoolRelation struct {
	ID                      int64     `db:"id"`
	AdminUserID             int       `db:"admin_user_id"`
	SchoolID                int       `db:"school_id"`
	SchoolName              string    `db:"school_name"`
	CommissionRate          float64   `db:"commission_rate"`
	IsOwner                 bool      `db:"is_owner"`
	PendingSettlementAmount int64     `db:"-"`
	CreatedAt               time.Time `db:"created_at"`
	UpdatedAt               time.Time `db:"updated_at"`
}
