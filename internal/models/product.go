package models

import (
	"time"

	"github.com/kuaizu-team/kuaizu-service/api"
)

// Product represents a product in the database
type Product struct {
	ID          int       `db:"id"`
	Name        string    `db:"name"`
	Type        int       `db:"type"` // 类型: 1-虚拟币, 2-服务权益
	Description *string   `db:"description"`
	Price       float64   `db:"price"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

// ToVO converts Product to API ProductVO
func (p *Product) ToVO() *api.ProductVO {
	return &api.ProductVO{
		Id:          &p.ID,
		Name:        &p.Name,
		Type:        &p.Type,
		Description: p.Description,
		Price:       &p.Price,
	}
}
