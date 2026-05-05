package service

import (
	"context"
	"strings"
	"sync"

	"caichip/internal/biz"
	"caichip/internal/data"

	"gorm.io/gorm"
)

// mergeMpnKeysForBOMLine 与报价缓存读路径一致：主 MPN + 可选替代料（去重、规范化）。
func mergeMpnKeysForBOMLine(line data.BomSessionLine) []string {
	seen := make(map[string]struct{})
	var keys []string
	add := func(raw string) {
		k := biz.NormalizeMPNForBOMSearch(strings.TrimSpace(raw))
		if k == "" {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	add(line.Mpn)
	if sub := strings.TrimSpace(derefStrPtr(line.SubstituteMpn)); sub != "" && !strings.EqualFold(sub, strings.TrimSpace(line.Mpn)) {
		add(sub)
	}
	return keys
}

// loadMfrReviewQuoteItemViaCacheRead 当 t_bom_quote_item 未直挂 session_id 时，按缓存合并键与会话业务日/平台解析子行（与 ListBomQuoteItemsForSessionLineRead 一致）。
func (s *BomService) loadMfrReviewQuoteItemViaCacheRead(ctx context.Context, sessionID string, quoteItemID uint64) (*biz.MfrReviewQuoteItem, error) {
	if quoteItemID == 0 || s.search == nil || !s.search.DBOk() {
		return nil, gorm.ErrRecordNotFound
	}
	view, lines, plats, err := s.loadSessionLinesAndPlatforms(ctx, sessionID, nil)
	if err != nil {
		return nil, err
	}
	tasks := mfrReadLineTasks(lines)
	if len(tasks) == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	var mu sync.Mutex
	var found *biz.MfrReviewQuoteItem
	runErr := parallelMfrReadLineTasks(ctx, tasks, func(workCtx context.Context, t mfrReadLineTask) (bool, error) {
		mu.Lock()
		if found != nil {
			mu.Unlock()
			return false, nil
		}
		mu.Unlock()

		rows, err := s.search.ListBomQuoteItemsForSessionLineRead(workCtx, sessionID, t.line.ID, view.BizDate, t.mk, plats)
		if err != nil {
			return false, err
		}
		for i := range rows {
			if rows[i].ItemID != quoteItemID {
				continue
			}
			it := mfrReviewQuoteItemFromBomQuoteReadRow(rows[i], t.line.ID)
			mu.Lock()
			if found == nil {
				found = it
			}
			mu.Unlock()
			return true, nil
		}
		return false, nil
	})
	if runErr != nil {
		return nil, runErr
	}
	if found != nil {
		return found, nil
	}
	return nil, gorm.ErrRecordNotFound
}

// listMfrReviewPendingQuoteItemsMerged 合并：直挂 session 的 pending 子行 + 经 quote_cache 关联且仍待审的子行。
func (s *BomService) listMfrReviewPendingQuoteItemsMerged(ctx context.Context, sessionID string, view *biz.BOMSessionView, lines []data.BomSessionLine, plats []string) ([]biz.MfrReviewQuoteItem, error) {
	seen := make(map[uint64]struct{})
	pending := make([]biz.MfrReviewQuoteItem, 0)
	var appendMu sync.Mutex
	appendPending := func(it biz.MfrReviewQuoteItem) {
		appendMu.Lock()
		defer appendMu.Unlock()
		if _, ok := seen[it.ID]; ok {
			return
		}
		seen[it.ID] = struct{}{}
		pending = append(pending, it)
	}
	if s.search != nil && s.search.DBOk() {
		tasks := mfrReadLineTasks(lines)
		if len(tasks) > 0 {
			err := parallelMfrReadLineTasks(ctx, tasks, func(workCtx context.Context, t mfrReadLineTask) (bool, error) {
				readRows, err := s.search.ListBomQuoteItemsForSessionLineRead(workCtx, sessionID, t.line.ID, view.BizDate, t.mk, plats)
				if err != nil {
					return false, err
				}
				appendPendingMfrFromBOMQuoteReadRows(readRows, t.line.ID, appendPending)
				return false, nil
			})
			if err != nil {
				return nil, err
			}
		}
	}
	return pending, nil
}
