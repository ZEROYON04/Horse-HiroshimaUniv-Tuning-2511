package service

import (
	"context"
	"fmt"
	"log"
	"sync"

	"backend/internal/model"
	"backend/internal/repository"

	"go.opentelemetry.io/otel"
)

type ProductService struct {
	store *repository.Store
}

func NewProductService(store *repository.Store) *ProductService {
	return &ProductService{store: store}
}

// forループを並列処理に変更
// 参考文献：https://developer.mamezou-tech.com/blogs/2025/01/10/go-conc/
func (s *ProductService) CreateOrders(ctx context.Context, userID int, items []model.RequestItem) ([]string, error) {
	var insertedOrderIDs []string

	ctx, span := otel.Tracer("service.product").Start(ctx, "ProductService.CreateOrders")
	defer span.End()

	err := s.store.ExecTx(ctx, func(txStore *repository.Store) error {
		itemsToProcess := make(map[int]int)
		for _, item := range items {
			if item.Quantity > 0 {
				itemsToProcess[item.ProductID] = item.Quantity
			}
		}
		if len(itemsToProcess) == 0 {
			return nil
		}
		// 追加1：排他制御用のMutex
		var mu sync.Mutex
		// 追加2：ゴルーチンの完了を待つためのWaitGroup
		var wg sync.WaitGroup
		// 追加3：最初のエラーを保存する変数
		var first_Err error
		// 追加4：エラーの排他制御用のMutex
		var errMu sync.Mutex

		for pID, quantity := range itemsToProcess {
			// 追加5：WaitingGroupを１つ増やす
			wg.Add(1)
			// 追加6：ゴルーチンを起動
			go func(productID int, qty int) {
				// 追加7：ゴルーチンが終了したらDoneを呼ぶ
				defer wg.Done()

				for i := 0; i < qty; i++ {
					// 追加8：エラーが発生している場合は処理をスキップ
					errMu.Lock()
					if first_Err != nil {
						// 追加9：エラーマップの更新を排他制御
						errMu.Unlock()
						return
					}
					// 追加9：エラーマップの更新を排他制御
					errMu.Unlock()

					order := &model.Order{
						UserID:    userID,
						ProductID: productID,
					}
					orderID, err := txStore.OrderRepo.Create(ctx, order)
					if err != nil {
						// 追加8：エラーが発生している場合は処理をスキップ
						errMu.Lock()
						if first_Err == nil {
							first_Err = fmt.Errorf("failed to create order for product %d: %w", productID, err)
						}
						// 追加9：エラーマップの更新を排他制御
						errMu.Unlock()
						return
					}

					// 追加10：マップの更新を排他制御
					mu.Lock()
					insertedOrderIDs = append(insertedOrderIDs, orderID)
					// 追加11：マップの更新を排他制御
					mu.Unlock()
				}
			}(pID, quantity)
		}
		// 追加12：全てのゴルーチンが終了するまで待つ
		wg.Wait()
		return first_Err
	})

	if err != nil {
		return nil, err
	}
	log.Printf("Created %d orders for user %d", len(insertedOrderIDs), userID)
	return insertedOrderIDs, nil
}

func (s *ProductService) FetchProducts(ctx context.Context, userID int, req model.ListRequest) ([]model.Product, int, error) {
	products, total, err := s.store.ProductRepo.ListProducts(ctx, userID, req)
	return products, total, err
}
