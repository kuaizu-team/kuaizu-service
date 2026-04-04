package handler

import (
	"log"

	"github.com/labstack/echo/v4"
	"github.com/kuaizu-team/kuaizu-service/api"
)

// ListProducts handles GET /products
func (s *Server) ListProducts(ctx echo.Context) error {
	products, err := s.repo.Product.GetAll(ctx.Request().Context())
	if err != nil {
		log.Printf("ListProducts error: %v", err)
		return InternalError(ctx, "获取商品列表失败")
	}

	// Convert to VOs
	var productVOs []api.ProductVO
	for _, product := range products {
		productVOs = append(productVOs, *product.ToVO())
	}

	return Success(ctx, productVOs)
}

// GetProductDetail handles GET /products/{id}
func (s *Server) GetProductDetail(ctx echo.Context, id int) error {
	product, err := s.repo.Product.GetByID(ctx.Request().Context(), id)
	if err != nil {
		log.Printf("GetProductDetail error: %v", err)
		return InternalError(ctx, "获取商品详情失败")
	}
	if product == nil {
		return NotFound(ctx, "商品不存在")
	}

	return Success(ctx, product.ToVO())
}
