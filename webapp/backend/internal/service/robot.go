package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service/utils"
	"context"
	"log"
)

type RobotService struct {
	store *repository.Store
}

func NewRobotService(store *repository.Store) *RobotService {
	return &RobotService{store: store}
}

// 注意：このメソッドは、現在、ordersテーブルのshipped_statusが"shipping"になっている注文"全件"を対象に配送計画を立てます。
// 注文の取得件数を制限した場合、ペナルティの対象になります。
func (s *RobotService) GenerateDeliveryPlan(ctx context.Context, robotID string, capacity int) (*model.DeliveryPlan, error) {
	var plan model.DeliveryPlan

	err := utils.WithTimeout(ctx, func(ctx context.Context) error {
		return s.store.ExecTx(ctx, func(txStore *repository.Store) error {
			orders, err := txStore.OrderRepo.GetShippingOrders(ctx)
			if err != nil {
				return err
			}
			plan, err = selectOrdersForDelivery(ctx, orders, robotID, capacity)
			if err != nil {
				return err
			}
			if len(plan.Orders) > 0 {
				orderIDs := make([]int64, len(plan.Orders))
				for i, order := range plan.Orders {
					orderIDs[i] = order.OrderID
				}

				if err := txStore.OrderRepo.UpdateStatuses(ctx, orderIDs, "delivering"); err != nil {
					return err
				}
				log.Printf("Updated status to 'delivering' for %d orders", len(orderIDs))
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

func (s *RobotService) UpdateOrderStatus(ctx context.Context, orderID int64, newStatus string) error {
	return utils.WithTimeout(ctx, func(ctx context.Context) error {
		return s.store.OrderRepo.UpdateStatuses(ctx, []int64{orderID}, newStatus)
	})
}

func selectOrdersForDelivery(ctx context.Context, orders []model.Order, robotID string, robotCapacity int) (model.DeliveryPlan, error) {

	selected, bestValue, totalWeight, err := dp_1dim(ctx, orders, robotCapacity)
	if err != nil {
		return model.DeliveryPlan{}, err
	}

	return model.DeliveryPlan{
		RobotID:     robotID,
		TotalWeight: totalWeight,
		TotalValue:  bestValue,
		Orders:      selected,
	}, nil
}

func dp_1dim(ctx context.Context, orders []model.Order, capacity int) ([]model.Order, int, int, error) {
	n := len(orders)
	if n == 0 {
		return nil, 0, 0, nil
	}

	// DP 配列
	dp := make([]int, capacity+1)

	// choice[i][w] = 注文 i を選んだか
	choice := make([][]bool, n)
	for i := range choice {
		choice[i] = make([]bool, capacity+1)
	}

	// --- DP 本体 ---
	for i := 0; i < n; i++ {
		weight := orders[i].Weight
		value := orders[i].Value

		for w := capacity; w >= weight; w-- {

			// ctx キャンセルチェック
			if w%256 == 0 {
				select {
				case <-ctx.Done():
					return nil, 0, 0, ctx.Err()
				default:
				}
			}

			if dp[w-weight]+value > dp[w] {
				dp[w] = dp[w-weight] + value
				choice[i][w] = true
			}
		}
	}

	// 最適値探索
	bestValue := 0
	bestWeight := 0
	for w := 0; w <= capacity; w++ {
		if dp[w] > bestValue {
			bestValue = dp[w]
			bestWeight = w
		}
	}

	// --- 復元 ---
	w := bestWeight
	var selected []model.Order

	for i := n - 1; i >= 0; i-- {
		if w >= orders[i].Weight && choice[i][w] {
			selected = append(selected, orders[i])
			w -= orders[i].Weight
		}
	}

	// 逆順修正
	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}

	// 重量合計
	totalWeight := 0
	for _, o := range selected {
		totalWeight += o.Weight
	}

	return selected, bestValue, totalWeight, nil
}
