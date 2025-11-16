package service

import (
	"context"
	"log"

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

func (s *ProductService) CreateOrders(ctx context.Context, userID int, items []model.RequestItem) ([]int, error) {
	var insertedOrderIDs []int

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
		var txCreateErr error
		insertedOrderIDs, txCreateErr = txStore.OrderRepo.Create(ctx, itemsToProcess, userID)
		if txCreateErr != nil {
			return txCreateErr
		}
		return nil
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
