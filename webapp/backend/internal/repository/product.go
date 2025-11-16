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
	// WHERE句と引数を先に構築
	var args []interface{}
	var whereClause string
	if req.Search != "" {
		whereClause += " WHERE MATCH (name,description) AGAINST (? IN BOOLEAN MODE)"
		args = append(args, req.Search)
	}

	// COUNTクエリを構築・実行する
	totalQuery := "SELECT COUNT(*) FROM products" + whereClause
	totalQuery = r.db.Rebind(totalQuery)

	var total int
	err := r.db.GetContext(ctx, &total, totalQuery, args...)
	if err != nil {
		return nil, 0, err
	}

	// 検索結果が0件なら、商品取得クエリは実行せずに即時リターン
	if total == 0 {
		return []model.Product{}, 0, nil
	}

	// 商品一覧クエリを構築・実行する (WHERE, ORDER, Pagingを適用)
	baseQuery := `
		SELECT product_id, name, value, weight, image, description
		FROM products
	` + whereClause // WHERE句を適用

	// ORDER BY と LIMIT/OFFSET を追加
	baseQuery += " ORDER BY " + req.SortField + " " + req.SortOrder + " , product_id ASC"
	baseQuery += " LIMIT " + strconv.Itoa(req.PageSize)
	baseQuery += " OFFSET " + strconv.Itoa(req.Offset)

	// Rebindを使って '?' を '$1' などに置換する
	baseQuery = r.db.Rebind(baseQuery)

	var products []model.Product
	err = r.db.SelectContext(ctx, &products, baseQuery, args...)
	if err != nil {
		return nil, 0, err
	}

	return products, total, nil
}
