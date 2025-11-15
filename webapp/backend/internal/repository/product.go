package repository

import (
	"backend/internal/model"
	"context"
	"strconv"
)

type ProductRepository struct {
	db DBTX
}

func NewProductRepository(db DBTX) *ProductRepository {
	return &ProductRepository{db: db}
}

// 商品一覧をクエリで指定された箇所のみ取得する
func (r *ProductRepository) ListProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	var products []model.Product
	baseQuery := `
		SELECT product_id, name, value, weight, image, description
		FROM products
	`
	args := []interface{}{}

	if req.Search != "" {
		baseQuery += " WHERE (name LIKE ? OR description LIKE ?)"
		searchPattern := "%" + req.Search + "%"
		args = append(args, searchPattern, searchPattern)
	}

	baseQuery += " ORDER BY " + req.SortField + " " + req.SortOrder + " , product_id ASC"
	baseQuery += " LIMIT " + strconv.Itoa(req.PageSize)
	baseQuery += " OFFSET " + strconv.Itoa(req.Offset)

	err := r.db.SelectContext(ctx, &products, baseQuery, args...)
	if err != nil {
		return nil, 0, err
	}

	// COUNT関数による件数の取得
	args = []interface{}{}
	var total int
	totalQuery := `SELECT COUNT(*) FROM products`

	err = r.db.GetContext(ctx, &total, totalQuery)
	if err != nil {
		return nil, 0, err
	}

	return products, total, nil
}
