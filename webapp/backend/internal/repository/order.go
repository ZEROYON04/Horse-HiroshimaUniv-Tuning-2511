package repository

import (
	"backend/internal/model"
	"context"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/otel"
)

type OrderRepository struct {
	db DBTX
}

func NewOrderRepository(db DBTX) *OrderRepository {
	return &OrderRepository{db: db}
}

// 注文を作成し、生成された注文IDを返す
func (r *OrderRepository) Create(ctx context.Context, itemsToProcess map[int]int, userID int) ([]int, error) {
	ctx, span := otel.Tracer("service.order").Start(ctx, "OrderRepository.Create")
	defer span.End()
	var insertedOrderIDs []int

	totalOrders := 0
	for _, count := range itemsToProcess {
		totalOrders += count
	}

	if totalOrders == 0 {
		return []int{}, nil
	}

	// 1. クエリの動的生成
	baseQuery := `INSERT INTO orders (user_id, product_id, shipped_status, created_at) VALUES `
	placeholderTemplate := `(?, ?, 'shipping', NOW())`

	// 挿入する VALUES のセットをカンマで区切って格納するためのスライス
	// 各セットは (user_id, product_id, 'shipping', NOW()) に対応
	placeholders := make([]string, 0, totalOrders)

	// 2. 引数リストの作成（フラット化）
	// 各注文につき2つの引数 (user_id, product_id) が必要
	args := make([]interface{}, 0, totalOrders*2)

	// itemsToProcess マップをループ
	for productID, count := range itemsToProcess {
		insertedOrderIDs = append(insertedOrderIDs, productID)
		for i := 0; i < count; i++ {
			// プレースホルダのセットを追加
			placeholders = append(placeholders, placeholderTemplate)

			// 対応する引数 (userID と productID) を追加
			args = append(args, userID)
			args = append(args, productID)
		}
	}

	// 3. クエリ文字列の結合
	// placeholders スライスをカンマとスペースで結合
	finalQuery := baseQuery + strings.Join(placeholders, ", ")
	// 4. クエリの実行
	result, err := r.db.ExecContext(ctx, finalQuery, args...)
	if err != nil {
		return nil, err
	}
	lastInsertID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	// 5. 挿入された注文IDの計算
	for i := 0; i < totalOrders; i++ {
		insertedOrderIDs[i] = int(lastInsertID) + i
	}

	return insertedOrderIDs, nil
}

// 複数の注文IDのステータスを一括で更新
// 主に配送ロボットが注文を引き受けた際に一括更新をするために使用
func (r *OrderRepository) UpdateStatuses(ctx context.Context, orderIDs []int64, newStatus string) error {
	ctx, span := otel.Tracer("repository.order").Start(ctx, "OrderRepository.UpdateStatuses")
	defer span.End()
	if len(orderIDs) == 0 {
		return nil
	}
	query, args, err := sqlx.In("UPDATE orders SET shipped_status = ? WHERE order_id IN (?)", newStatus, orderIDs)
	if err != nil {
		return err
	}
	query = r.db.Rebind(query)
	_, err = r.db.ExecContext(ctx, query, args...)
	return err
}

// 配送中(shipped_status:shipping)の注文一覧を取得
func (r *OrderRepository) GetShippingOrders(ctx context.Context) ([]model.Order, error) {
	var orders []model.Order
	ctx, span := otel.Tracer("repository.order").Start(ctx, "OrderRepository.GetShippingOrders")
	defer span.End()
	query := `
        SELECT
            o.order_id,
            p.weight,
            p.value
        FROM orders o
        JOIN products p ON o.product_id = p.product_id
        WHERE o.shipped_status = 'shipping'
    `
	err := r.db.SelectContext(ctx, &orders, query)
	return orders, err
}

// 注文履歴一覧を取得
func (r *OrderRepository) ListOrders(ctx context.Context, userID int, req model.ListRequest) ([]model.Order, int, error) {
	query := `
        SELECT t1.order_id, t1.product_id, t2.name as product_name, t1.shipped_status, t1.created_at, t1.arrived_at
        FROM orders t1
        INNER JOIN products t2 ON t1.product_id = t2.product_id
        WHERE t1.user_id = ?;
    `

	var ordersRaw []model.Order
	if err := r.db.SelectContext(ctx, &ordersRaw, query, userID); err != nil {
		return nil, 0, err
	}

	var orders []model.Order
	for _, o := range ordersRaw {

		if req.Search != "" {
			if req.Type == "prefix" {
				if !strings.HasPrefix(o.ProductName, req.Search) {
					continue
				}
			} else {
				if !strings.Contains(o.ProductName, req.Search) {
					continue
				}
			}
		}
		orders = append(orders, model.Order{
			OrderID:       o.OrderID,
			ProductID:     o.ProductID,
			ProductName:   o.ProductName,
			ShippedStatus: o.ShippedStatus,
			CreatedAt:     o.CreatedAt,
			ArrivedAt:     o.ArrivedAt,
		})
	}

	switch req.SortField {
	case "product_name":
		if strings.ToUpper(req.SortOrder) == "DESC" {
			sort.SliceStable(orders, func(i, j int) bool {
				return orders[i].ProductName > orders[j].ProductName
			})
		} else {
			sort.SliceStable(orders, func(i, j int) bool {
				return orders[i].ProductName < orders[j].ProductName
			})
		}
	case "created_at":
		if strings.ToUpper(req.SortOrder) == "DESC" {
			sort.SliceStable(orders, func(i, j int) bool {
				return orders[i].CreatedAt.After(orders[j].CreatedAt)
			})
		} else {
			sort.SliceStable(orders, func(i, j int) bool {
				return orders[i].CreatedAt.Before(orders[j].CreatedAt)
			})
		}
	case "shipped_status":
		if strings.ToUpper(req.SortOrder) == "DESC" {
			sort.SliceStable(orders, func(i, j int) bool {
				return orders[i].ShippedStatus > orders[j].ShippedStatus
			})
		} else {
			sort.SliceStable(orders, func(i, j int) bool {
				return orders[i].ShippedStatus < orders[j].ShippedStatus
			})
		}
	case "arrived_at":
		if strings.ToUpper(req.SortOrder) == "DESC" {
			sort.SliceStable(orders, func(i, j int) bool {
				if orders[i].ArrivedAt.Valid && orders[j].ArrivedAt.Valid {
					return orders[i].ArrivedAt.Time.After(orders[j].ArrivedAt.Time)
				}
				return orders[i].ArrivedAt.Valid
			})
		} else {
			sort.SliceStable(orders, func(i, j int) bool {
				if orders[i].ArrivedAt.Valid && orders[j].ArrivedAt.Valid {
					return orders[i].ArrivedAt.Time.Before(orders[j].ArrivedAt.Time)
				}
				return orders[j].ArrivedAt.Valid
			})
		}
	case "order_id":
		fallthrough
	default:
		if strings.ToUpper(req.SortOrder) == "DESC" {
			sort.SliceStable(orders, func(i, j int) bool {
				return orders[i].OrderID > orders[j].OrderID
			})
		} else {
			sort.SliceStable(orders, func(i, j int) bool {
				return orders[i].OrderID < orders[j].OrderID
			})
		}
	}

	total := len(orders)
	start := req.Offset
	end := req.Offset + req.PageSize
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	pagedOrders := orders[start:end]

	return pagedOrders, total, nil
}
