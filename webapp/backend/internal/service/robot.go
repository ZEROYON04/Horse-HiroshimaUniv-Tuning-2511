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
			plan, err = dpSelectOrders(ctx, orders, robotID, capacity)
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

func dpSelectOrders(ctx context.Context, orders []model.Order, robotID string, robotCapacity int) (model.DeliveryPlan, error) {
	n := len(orders)
	if n == 0 || robotCapacity <= 0 {
		return model.DeliveryPlan{
			RobotID:     robotID,
			TotalWeight: 0,
			TotalValue:  0,
			Orders:      nil,
		}, nil
	}

	// values[i][w] = max value using first i items with capacity w
	values := make([][]int, n+1)
	for i := 0; i <= n; i++ {
		values[i] = make([]int, robotCapacity+1)
	}

	for i := 1; i <= n; i++ {
		// キャンセルチェック
		select {
		case <-ctx.Done():
			return model.DeliveryPlan{}, ctx.Err()
		default:
		}
		w := orders[i-1].Weight
		v := orders[i-1].Value
		for cap := 0; cap <= robotCapacity; cap++ {
			if w > cap {
				values[i][cap] = values[i-1][cap]
			} else {
				without := values[i-1][cap]
				with := values[i-1][cap-w] + v
				if with > without {
					values[i][cap] = with
				} else {
					values[i][cap] = without
				}
			}
		}
	}

	bestValue := values[n][robotCapacity]

	// 復元
	cap := robotCapacity
	var bestSet []model.Order
	for i := n; i > 0 && cap >= 0; i-- {
		if values[i][cap] != values[i-1][cap] {
			bestSet = append(bestSet, orders[i-1])
			cap -= orders[i-1].Weight
		}
	}
	// 逆順に入っているので戻す
	for i, j := 0, len(bestSet)-1; i < j; i, j = i+1, j-1 {
		bestSet[i], bestSet[j] = bestSet[j], bestSet[i]
	}

	totalWeight := 0
	for _, o := range bestSet {
		totalWeight += o.Weight
	}

	return model.DeliveryPlan{
		RobotID:     robotID,
		TotalWeight: totalWeight,
		TotalValue:  bestValue,
		Orders:      bestSet,
	}, nil
}
