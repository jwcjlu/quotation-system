package service

import (
	"context"
	"sort"
	"strings"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/data"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ListMatchSources 返回会话各行 × 勾选平台的报价缓存摘要；不要求 BOM_NOT_READY，便于排障。
func (s *BomService) ListMatchSources(ctx context.Context, req *v1.ListMatchSourcesRequest) (*v1.ListMatchSourcesReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sid, err := parseBomSessionID(req.GetBomId())
	if err != nil {
		return nil, err
	}
	view, lines, plats, err := s.loadSessionLinesAndPlatforms(ctx, sid, nil)
	if err != nil {
		return nil, err
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].LineNo < lines[j].LineNo })
	pairList := dedupeQuoteCachePairs(lines, plats)
	cacheMap, err := s.search.LoadQuoteCachesForKeys(ctx, view.BizDate, pairList)
	if err != nil {
		return nil, err
	}
	outLines := make([]*v1.MatchSourceLineRow, 0, len(lines))
	for _, line := range lines {
		mergeKey := biz.NormalizeMPNForBOMSearch(line.Mpn)
		pls := make([]*v1.MatchSourcePlatformRow, 0, len(plats))
		for _, pid := range plats {
			snap := cacheMap[quoteCachePairKey(mergeKey, pid)]
			hit := snap != nil
			var skip string
			if hit && quoteCacheUsable(snap) {
				skip = ""
			} else {
				skip = quoteCacheUnusableReason(hit, snap)
			}
			oc := ""
			if snap != nil {
				oc = snap.Outcome
			}
			sz := int32(0)
			if snap != nil {
				sz = int32(len(snap.QuotesJSON))
			}
			pls = append(pls, &v1.MatchSourcePlatformRow{
				Platform:       pid,
				CacheHit:       hit,
				SkipReason:     skip,
				Outcome:        oc,
				QuotesJsonSize: sz,
			})
		}
		outLines = append(outLines, &v1.MatchSourceLineRow{
			LineNo:             int32(line.LineNo),
			Mpn:                line.Mpn,
			MergeMpn:           mergeKey,
			Quantity:           int32(bomLineQtyInt(line.Qty)),
			DemandManufacturer: derefStrPtr(line.Mfr),
			DemandPackage:      derefStrPtr(line.Package),
			Platforms:          pls,
		})
	}
	return &v1.ListMatchSourcesReply{
		BizDate:          view.BizDate.Format("2006-01-02"),
		SessionPlatforms: append([]string(nil), plats...),
		Lines:            outLines,
	}, nil
}

// GetMatchSourceDetail 返回单行单平台下的缓存原文与跳过原因。
func (s *BomService) GetMatchSourceDetail(ctx context.Context, req *v1.GetMatchSourceDetailRequest) (*v1.GetMatchSourceDetailReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sid, err := parseBomSessionID(req.GetBomId())
	if err != nil {
		return nil, err
	}
	wantLine := int(req.GetLineNo())
	if wantLine <= 0 {
		return nil, kerrors.BadRequest("BAD_LINE_NO", "line_no must be positive")
	}
	view, lines, plats, err := s.loadSessionLinesAndPlatforms(ctx, sid, nil)
	if err != nil {
		return nil, err
	}
	var line *data.BomSessionLine
	for i := range lines {
		if lines[i].LineNo == wantLine {
			line = &lines[i]
			break
		}
	}
	if line == nil {
		return nil, kerrors.NotFound("LINE_NOT_FOUND", "session line not found for line_no")
	}
	pid := biz.NormalizePlatformID(strings.TrimSpace(req.GetPlatform()))
	if pid == "" {
		return nil, kerrors.BadRequest("BAD_PLATFORM", "platform required")
	}
	allowed := false
	for _, p := range plats {
		if p == pid {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, kerrors.BadRequest("PLATFORM_NOT_IN_SESSION", "platform not selected on session")
	}
	mergeKey := biz.NormalizeMPNForBOMSearch(line.Mpn)
	snap, hit, err := s.search.LoadQuoteCacheByMergeKey(ctx, mergeKey, pid, view.BizDate)
	if err != nil {
		return nil, err
	}
	var skip string
	if hit && quoteCacheUsable(snap) {
		skip = ""
	} else {
		skip = quoteCacheUnusableReason(hit, snap)
	}
	oc := ""
	qj := ""
	nd := ""
	if snap != nil {
		oc = snap.Outcome
		qj = string(snap.QuotesJSON)
		nd = string(snap.NoMpnDetail)
	}
	return &v1.GetMatchSourceDetailReply{
		MergeMpn:              mergeKey,
		Platform:              pid,
		CacheHit:              hit,
		SkipReason:            skip,
		Outcome:               oc,
		NoMpnDetail:           nd,
		QuotesJson:            qj,
		QuoteRowEvals:         nil,
		BomDemandMpn:          line.Mpn,
		BomDemandPackage:      derefStrPtr(line.Package),
		BomDemandManufacturer: derefStrPtr(line.Mfr),
	}, nil
}

// GetBomLineQuoteItems 返回会话行原始需求与 t_bom_quote_item 明细（经 t_bom_quote_cache 按合并键+平台关联）。
func (s *BomService) GetBomLineQuoteItems(ctx context.Context, req *v1.GetBomLineQuoteItemsRequest) (*v1.GetBomLineQuoteItemsReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sid, err := parseBomSessionID(req.GetBomId())
	if err != nil {
		return nil, err
	}
	wantLine := int(req.GetLineNo())
	if wantLine <= 0 {
		return nil, kerrors.BadRequest("BAD_LINE_NO", "line_no must be positive")
	}
	view, lines, plats, err := s.loadSessionLinesAndPlatforms(ctx, sid, nil)
	if err != nil {
		return nil, err
	}
	var line *data.BomSessionLine
	for i := range lines {
		if lines[i].LineNo == wantLine {
			line = &lines[i]
			break
		}
	}
	if line == nil {
		return nil, kerrors.NotFound("LINE_NOT_FOUND", "session line not found for line_no")
	}
	mergeKey := biz.NormalizeMPNForBOMSearch(line.Mpn)
	items, err := s.search.ListBomQuoteItemsForSessionLineRead(ctx, sid, line.ID, view.BizDate, mergeKey, plats)
	if err != nil {
		return nil, err
	}
	demand := &v1.BomLineDemandSnapshot{
		LineNo:             int32(line.LineNo),
		LineDbId:           line.ID,
		RawText:            derefStrPtr(line.RawText),
		Mpn:                line.Mpn,
		UnifiedMpn:         derefStrPtr(line.UnifiedMpn),
		ReferenceDesignator: derefStrPtr(line.ReferenceDesignator),
		SubstituteMpn:      derefStrPtr(line.SubstituteMpn),
		Remark:             derefStrPtr(line.Remark),
		Description:        derefStrPtr(line.Description),
		DemandManufacturer: derefStrPtr(line.Mfr),
		ExtraJson:          string(line.ExtraJSON),
	}
	if line.ManufacturerCanonicalID != nil {
		demand.ManufacturerCanonicalId = strings.TrimSpace(*line.ManufacturerCanonicalID)
	}
	if line.Package != nil {
		demand.DemandPackage = strings.TrimSpace(*line.Package)
	}
	if line.Qty != nil {
		demand.Quantity = *line.Qty
	}
	outItems := make([]*v1.BomQuoteItemRead, 0, len(items))
	for i := range items {
		it := &items[i]
		outItems = append(outItems, &v1.BomQuoteItemRead{
			Platform:                 it.PlatformID,
			QuoteId:                  it.QuoteID,
			ItemId:                   it.ItemID,
			Model:                    it.Model,
			Manufacturer:             it.Manufacturer,
			ManufacturerCanonicalId: it.ManufacturerCanonicalID,
			Package:                  it.Package,
			Stock:                    it.Stock,
			Desc:                     it.Desc,
			Moq:                      it.MOQ,
			LeadTime:                 it.LeadTime,
			PriceTiers:               it.PriceTiers,
			HkPrice:                  it.HKPrice,
			MainlandPrice:            it.MainlandPrice,
			QueryModel:               it.QueryModel,
			DatasheetUrl:             it.DatasheetURL,
			SourceType:               it.SourceType,
			SessionId:                it.SessionID,
			LineId:                   it.LineID,
		})
	}
	return &v1.GetBomLineQuoteItemsReply{
		BizDate:  view.BizDate.Format("2006-01-02"),
		MergeMpn: mergeKey,
		Demand:   demand,
		Items:    outItems,
	}, nil
}
