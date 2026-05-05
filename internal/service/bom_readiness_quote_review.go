package service

import (
	"context"
	"strconv"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/data"
)

// buildQuoteReviewReadinessPayload 计算会话内各需求行（t_bom_session_line）的规则 B / TopK/TopN 快照（SRS REQ-READY-001/004）。
// 每一行先合并会话勾选平台下该行的全部报价子行再计算；Top 粒度为需求行，非按平台拆分。
// 当 matchDepsOK() 为假时返回 (nil, 0, defaultTopN, nil)，不报错。
func (s *BomService) buildQuoteReviewReadinessPayload(ctx context.Context, view *biz.BOMSessionView, lines []data.BomSessionLine) ([]*v1.LineQuoteReviewReadiness, int32, int32, error) {
	cfg := biz.DefaultQuoteReviewConfig()
	topNEcho := int32(cfg.TopN)
	if !s.matchDepsOK() {
		return nil, 0, topNEcho, nil
	}
	sid := view.SessionID
	now := time.Now()
	dep := biz.QuoteReviewPriceDeps{
		BaseCCY:          s.bomMatchBaseCCY(),
		RoundingMode:     s.bomMatchRoundingMode(),
		ParseTierStrings: s.bomMatchParseTiers(),
		BizDate:          view.BizDate,
		RequestDay:       now,
		FX:               s.fx,
	}
	out := make([]*v1.LineQuoteReviewReadiness, 0, len(lines))
	var notOk int32
	for i := range lines {
		line := lines[i]
		dep.BomQty = bomLineQtyInt(line.Qty)
		mergeKey := biz.NormalizeMPNForBOMSearch(line.Mpn)
		readRows, err := s.search.ListBomQuoteItemsForSessionLineRead(ctx, sid, line.ID, view.BizDate, mergeKey, view.PlatformIDs)
		if err != nil {
			return nil, 0, 0, err
		}
		inputs, err := biz.BuildQuoteReviewRowInputs(ctx, readRows, dep)
		if err != nil {
			return nil, 0, 0, err
		}
		o := biz.ComputeQuoteReviewLineOutcome(cfg, inputs)
		if !o.RuleBOk {
			notOk++
		}
		out = append(out, &v1.LineQuoteReviewReadiness{
			LineId:                        strconv.FormatInt(line.ID, 10),
			LineQuoteReviewRuleBOk:        o.RuleBOk,
			LineQuoteReviewCandidatePoolM: int32(o.CandidatePoolM),
			LineQuoteReviewTopKItemIds:    append([]uint64(nil), o.TopKItemIDs...),
			LineQuoteReviewTopNItemIds:    append([]uint64(nil), o.TopNItemIDs...),
		})
	}
	return out, notOk, topNEcho, nil
}

// quoteItemMfrPhase2PriorityItemIDsByLineNo 返回按 line_no 索引的 TopK∪TopN 报价 item_id 集合；某行不在 map 中表示该行不限定（两池皆空）。
// matchDepsOK 为 false 时返回 (nil, nil)，调用方不做子集过滤。
func (s *BomService) quoteItemMfrPhase2PriorityItemIDsByLineNo(
	ctx context.Context,
	view *biz.BOMSessionView,
	lines []data.BomSessionLine,
	plats []string,
) (map[int]map[uint64]struct{}, error) {
	if !s.matchDepsOK() {
		return nil, nil
	}
	cfg := biz.DefaultQuoteReviewConfig()
	sid := view.SessionID
	now := time.Now()
	baseDep := biz.QuoteReviewPriceDeps{
		BaseCCY:          s.bomMatchBaseCCY(),
		RoundingMode:     s.bomMatchRoundingMode(),
		ParseTierStrings: s.bomMatchParseTiers(),
		BizDate:          view.BizDate,
		RequestDay:       now,
		FX:               s.fx,
	}
	out := make(map[int]map[uint64]struct{})
	for i := range lines {
		line := lines[i]
		dep := baseDep
		dep.BomQty = bomLineQtyInt(line.Qty)
		mergeKey := biz.NormalizeMPNForBOMSearch(line.Mpn)
		readRows, err := s.search.ListBomQuoteItemsForSessionLineRead(ctx, sid, line.ID, view.BizDate, mergeKey, plats)
		if err != nil {
			return nil, err
		}
		inputs, err := biz.BuildQuoteReviewRowInputs(ctx, readRows, dep)
		if err != nil {
			return nil, err
		}
		o := biz.ComputeQuoteReviewLineOutcome(cfg, inputs)
		if len(o.TopKItemIDs) == 0 && len(o.TopNItemIDs) == 0 {
			continue
		}
		set := make(map[uint64]struct{})
		for _, id := range o.TopKItemIDs {
			set[id] = struct{}{}
		}
		for _, id := range o.TopNItemIDs {
			set[id] = struct{}{}
		}
		out[line.LineNo] = set
	}
	return out, nil
}

// mergeQuoteItemMfrReviewsIntoReadiness 将阶段二列表写入 GetReadiness 响应（与 ListQuoteItemMfrReviews 同源）。
func (s *BomService) mergeQuoteItemMfrReviewsIntoReadiness(
	ctx context.Context,
	out *v1.GetReadinessReply,
	sessionID string,
	includeList bool,
	includeAll bool,
) error {
	if !includeList {
		return nil
	}
	core, err := s.listQuoteItemMfrReviewsInternal(ctx, sessionID, includeAll)
	if err != nil {
		return err
	}
	out.QuoteMfrReviewGateOpen = core.GateOpen
	out.AllPendingQuoteMfrCount = core.AllPendingQuoteMfrCount
	out.QuoteItemMfrReviewItems = make([]*v1.QuoteItemMfrReviewRow, 0, len(core.Items))
	for _, it := range core.Items {
		out.QuoteItemMfrReviewItems = append(out.QuoteItemMfrReviewItems, &v1.QuoteItemMfrReviewRow{
			QuoteItemId:                 it.QuoteItemID,
			LineNo:                      int32(it.LineNo),
			LineManufacturerCanonicalId: it.LineManufacturerCanonicalID,
			Manufacturer:                it.Manufacturer,
			PlatformId:                  "",
		})
	}
	return nil
}
