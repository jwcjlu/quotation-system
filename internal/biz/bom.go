package biz

import (
	"context"
	"fmt"
	"time"

	"caichip/pkg/parser"
)

// BOMUseCase BOM 业务用例
type BOMUseCase struct {
	repo BOMRepo
}

// BOMRepo BOM 仓储接口
type BOMRepo interface {
	SaveBOM(ctx context.Context, bom *BOM) error
	GetBOM(ctx context.Context, bomID string) (*BOM, error)
}

// NewBOMUseCase 创建 BOM 用例
func NewBOMUseCase(repo BOMRepo) *BOMUseCase {
	return &BOMUseCase{repo: repo}
}

// ParseAndSave 解析并保存 BOM
func (uc *BOMUseCase) ParseAndSave(ctx context.Context, data []byte, mode parser.ParseMode, mapping parser.ColumnMapping) (*BOM, error) {
	items, err := parser.Parse(ctx, data, mode, mapping)
	if err != nil {
		return nil, fmt.Errorf("parse bom: %w", err)
	}

	bomItems := make([]*BOMItem, len(items))
	for i, it := range items {
		bomItems[i] = &BOMItem{
			Index:        it.Index,
			Raw:          it.Raw,
			Model:        it.Model,
			Manufacturer: it.Manufacturer,
			Package:      it.Package,
			Quantity:     it.Quantity,
			Params:       it.Params,
		}
	}

	bom := &BOM{
		ID:        generateBOMID(),
		CreatedAt: time.Now(),
		Items:     bomItems,
	}

	if err := uc.repo.SaveBOM(ctx, bom); err != nil {
		return nil, fmt.Errorf("save bom: %w", err)
	}
	return bom, nil
}

// GetBOM 获取 BOM
func (uc *BOMUseCase) GetBOM(ctx context.Context, bomID string) (*BOM, error) {
	return uc.repo.GetBOM(ctx, bomID)
}

func generateBOMID() string {
	return fmt.Sprintf("bom_%d", time.Now().UnixNano())
}
