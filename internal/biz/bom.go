package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"caichip/pkg/parser"
)

// BOMUseCase BOM 业务用例
type BOMUseCase struct {
	repo        BOMRepo
	sessionRepo BOMSessionRepo
	taskEnsurer BOMSearchTaskEnsurer
}

type noopBOMSearchTaskEnsurer struct{}

func (noopBOMSearchTaskEnsurer) EnsureTasksForSession(ctx context.Context, sessionID string) error {
	_, _ = ctx, sessionID
	return nil
}

// BOMRepo BOM 仓储接口
type BOMRepo interface {
	SaveBOM(ctx context.Context, bom *BOM) error
	GetBOM(ctx context.Context, bomID string) (*BOM, error)
}

// NewBOMUseCase 创建 BOM 用例（sessionRepo 用于带 session_id 的上传写 bom_session_line）。
func NewBOMUseCase(repo BOMRepo, sessionRepo BOMSessionRepo, taskEnsurer BOMSearchTaskEnsurer) *BOMUseCase {
	if taskEnsurer == nil {
		taskEnsurer = noopBOMSearchTaskEnsurer{}
	}
	return &BOMUseCase{repo: repo, sessionRepo: sessionRepo, taskEnsurer: taskEnsurer}
}

// ParseAndSave 解析并保存 BOM。sessionID 非空时校验会话存在，写入 bom_session_line，且 bom.ID == sessionID。
func (uc *BOMUseCase) ParseAndSave(ctx context.Context, data []byte, mode parser.ParseMode, mapping parser.ColumnMapping, sessionID string) (*BOM, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID != "" {
		if _, err := uc.sessionRepo.GetByID(ctx, sessionID); err != nil {
			return nil, err
		}
	}

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

	bomID := generateBOMID()
	if sessionID != "" {
		bomID = sessionID
	}
	bom := &BOM{
		ID:        bomID,
		CreatedAt: time.Now(),
		Items:     bomItems,
	}

	if err := uc.repo.SaveBOM(ctx, bom); err != nil {
		return nil, fmt.Errorf("save bom: %w", err)
	}

	if sessionID != "" {
		lines := bomItemsToSessionLines(bomItems)
		if err := uc.sessionRepo.ReplaceSessionLines(ctx, sessionID, string(mode), lines); err != nil {
			return nil, err
		}
		_ = uc.taskEnsurer.EnsureTasksForSession(ctx, sessionID)
	}
	return bom, nil
}

func bomItemsToSessionLines(items []*BOMItem) []*BOMSessionLine {
	out := make([]*BOMSessionLine, 0, len(items))
	for i, it := range items {
		lineNo := i + 1
		mpn := strings.TrimSpace(it.Model)
		if mpn == "" {
			mpn = "-"
		}
		q := float64(it.Quantity)
		qty := &q
		var extra []byte
		if strings.TrimSpace(it.Params) != "" {
			extra, _ = json.Marshal(map[string]string{"params": it.Params})
		}
		out = append(out, &BOMSessionLine{
			LineNo:    lineNo,
			RawText:   it.Raw,
			MPN:       mpn,
			MFR:       strings.TrimSpace(it.Manufacturer),
			Package:   strings.TrimSpace(it.Package),
			Qty:       qty,
			ExtraJSON: extra,
		})
	}
	return out
}

// GetBOM 获取 BOM
func (uc *BOMUseCase) GetBOM(ctx context.Context, bomID string) (*BOM, error) {
	return uc.repo.GetBOM(ctx, bomID)
}

func generateBOMID() string {
	return fmt.Sprintf("bom_%d", time.Now().UnixNano())
}
