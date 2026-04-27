package models

import (
	"encoding/json"
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
	ConfigJSON  *string   `db:"config_json"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

// OliveBranchCount returns how many olive branches this product grants per purchase.
// Falls back to 1 if config_json is absent or doesn't specify a count.
func (p *Product) OliveBranchCount() int {
	if p.ConfigJSON == nil {
		return 1
	}
	var cfg struct {
		OliveBranchCount int `json:"olive_branch_count"`
	}
	if err := json.Unmarshal([]byte(*p.ConfigJSON), &cfg); err != nil || cfg.OliveBranchCount <= 0 {
		return 1
	}
	return cfg.OliveBranchCount
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
