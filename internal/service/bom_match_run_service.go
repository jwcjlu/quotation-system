package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/data"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/xuri/excelize/v2"
)

func (s *BomService) SaveMatchRun(ctx context.Context, req *v1.SaveMatchRunRequest) (*v1.SaveMatchRunReply, error) {
	view, lines, plats, err := s.loadSessionLinesAndPlatforms(ctx, req.GetSessionId(), nil)
	if err != nil {
		return nil, err
	}
	if err := s.matchReadinessError(ctx, req.GetSessionId(), view, lines); err != nil {
		return nil, err
	}
	items, _, err := s.computeMatchItems(ctx, view, lines, plats)
	if err != nil {
		return nil, err
	}
	openGaps, err := s.gaps.ListLineGaps(ctx, req.GetSessionId(), []string{
		biz.LineGapOpen, biz.LineGapManualQuoteAdded, biz.LineGapSubstituteSelected,
	})
	if err != nil {
		return nil, err
	}
	gapByLine := make(map[int64]biz.BOMLineGap, len(openGaps))
	for _, g := range openGaps {
		gapByLine[g.LineID] = g
	}
	drafts := make([]biz.BOMMatchResultItemDraft, 0, len(lines))
	for i, line := range lines {
		mi := items[i]
		raw, _ := json.Marshal(mi)
		gap := gapByLine[line.ID]
		source := biz.MatchResultSourceFromMatchStatus(mi.GetMatchStatus(), mi.GetPlatform() == "manual", gap.SubstituteMpn != "")
		if mi.GetMatchStatus() == "no_match" {
			source = biz.MatchResultUnresolved
		}
		drafts = append(drafts, matchItemToRunDraft(line, mi, source, gap, raw))
	}
	runID, runNo, err := s.matchRuns.CreateMatchRun(ctx, req.GetSessionId(), view.SelectionRevision, s.bomMatchBaseCCY(), "", drafts)
	if err != nil {
		return nil, err
	}
	_ = s.matchRuns.SupersedePreviousRuns(ctx, req.GetSessionId(), runID)
	return &v1.SaveMatchRunReply{RunId: strconv.FormatUint(runID, 10), RunNo: int32(runNo)}, nil
}

func (s *BomService) ListMatchRuns(ctx context.Context, req *v1.ListMatchRunsRequest) (*v1.ListMatchRunsReply, error) {
	runs, err := s.matchRuns.ListMatchRuns(ctx, req.GetSessionId())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.MatchRunListItem, 0, len(runs))
	for _, r := range runs {
		out = append(out, matchRunViewToProto(r))
	}
	return &v1.ListMatchRunsReply{Runs: out}, nil
}

func (s *BomService) GetMatchRun(ctx context.Context, req *v1.GetMatchRunRequest) (*v1.GetMatchRunReply, error) {
	runID, err := strconv.ParseUint(req.GetRunId(), 10, 64)
	if err != nil {
		return nil, kerrors.BadRequest("BAD_RUN_ID", "invalid run_id")
	}
	run, items, err := s.matchRuns.GetMatchRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.MatchRunResultItem, 0, len(items))
	for _, item := range items {
		out = append(out, matchRunItemToProto(item))
	}
	return &v1.GetMatchRunReply{Run: matchRunViewToProto(*run), Items: out}, nil
}

func matchItemToRunDraft(line data.BomSessionLine, mi *v1.MatchItem, source string, gap biz.BOMLineGap, raw []byte) biz.BOMMatchResultItemDraft {
	return biz.BOMMatchResultItemDraft{
		LineID: line.ID, LineNo: line.LineNo, SourceType: source, MatchStatus: mi.GetMatchStatus(),
		GapID: gap.ID, PlatformID: mi.GetPlatform(), DemandMpn: line.Mpn,
		DemandMfr: mi.GetDemandManufacturer(), DemandPackage: mi.GetDemandPackage(),
		DemandQty: float64(mi.GetQuantity()), MatchedMpn: mi.GetMatchedModel(),
		MatchedMfr: mi.GetManufacturer(), Stock: mi.GetStock(), LeadTime: mi.GetLeadTime(),
		UnitPrice: mi.GetUnitPrice(), Subtotal: mi.GetSubtotal(), OriginalMpn: gap.Mpn,
		SubstituteMpn: gap.SubstituteMpn, SubstituteReason: gap.SubstituteReason,
		CodeTS: mi.GetCodeTs(), ControlMark: mi.GetControlMark(),
		ImportTaxImpOrdinaryRate: mi.GetImportTaxImpOrdinaryRate(),
		ImportTaxImpDiscountRate: mi.GetImportTaxImpDiscountRate(),
		ImportTaxImpTempRate:     mi.GetImportTaxImpTempRate(), SnapshotJSON: raw,
	}
}

func matchRunViewToProto(r biz.BOMMatchRunView) *v1.MatchRunListItem {
	out := &v1.MatchRunListItem{
		RunId: strconv.FormatUint(r.ID, 10), RunNo: int32(r.RunNo), SessionId: r.SessionID,
		Status: r.Status, LineTotal: int32(r.LineTotal), MatchedLineCount: int32(r.MatchedLineCount),
		UnresolvedLineCount: int32(r.UnresolvedLineCount), TotalAmount: r.TotalAmount, Currency: r.Currency,
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
	}
	if r.SavedAt != nil {
		out.SavedAt = r.SavedAt.Format(time.RFC3339)
	}
	return out
}

func matchRunItemToProto(item biz.BOMMatchResultItemDraft) *v1.MatchRunResultItem {
	return &v1.MatchRunResultItem{
		LineId: strconv.FormatInt(item.LineID, 10), LineNo: int32(item.LineNo), SourceType: item.SourceType,
		MatchStatus: item.MatchStatus, GapId: formatUintIfNonZero(item.GapID),
		QuoteItemId: formatUintIfNonZero(item.QuoteItemID), PlatformId: item.PlatformID,
		DemandMpn: item.DemandMpn, MatchedMpn: item.MatchedMpn, MatchedMfr: item.MatchedMfr,
		MatchedPackage: item.MatchedPackage, Stock: item.Stock, LeadTime: item.LeadTime,
		UnitPrice: item.UnitPrice, Subtotal: item.Subtotal, Currency: item.Currency,
		SubstituteMpn: item.SubstituteMpn, SubstituteReason: item.SubstituteReason,
		CodeTs: item.CodeTS, ControlMark: item.ControlMark,
		ImportTaxImpOrdinaryRate: item.ImportTaxImpOrdinaryRate,
		ImportTaxImpDiscountRate: item.ImportTaxImpDiscountRate,
		ImportTaxImpTempRate:     item.ImportTaxImpTempRate,
	}
}

func (s *BomService) exportMatchRunItems(items []biz.BOMMatchResultItemDraft, format string) (*v1.ExportSessionReply, error) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	headers := []any{"行号", "需求型号", "需求厂牌", "需求封装", "需求数量", "来源", "匹配状态", "平台", "匹配型号", "匹配厂牌", "匹配封装", "库存", "交期", "单价", "小计", "币种", "替代料", "替代原因", "税号", "监管条件", "普通税率", "优惠税率", "暂定税率"}
	_ = f.SetSheetRow(sheet, "A1", &headers)
	for i, item := range items {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		_ = f.SetSheetRow(sheet, cell, &[]any{
			item.LineNo, item.DemandMpn, item.DemandMfr, item.DemandPackage, item.DemandQty,
			item.SourceType, item.MatchStatus, item.PlatformID, item.MatchedMpn, item.MatchedMfr,
			item.MatchedPackage, item.Stock, item.LeadTime, item.UnitPrice, item.Subtotal, item.Currency,
			item.SubstituteMpn, item.SubstituteReason, item.CodeTS, item.ControlMark,
			item.ImportTaxImpOrdinaryRate, item.ImportTaxImpDiscountRate, item.ImportTaxImpTempRate,
		})
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	fn := "bom_match_run_export.xlsx"
	if strings.EqualFold(strings.TrimSpace(format), "csv") {
		fn = "bom_match_run_export.csv"
	}
	return &v1.ExportSessionReply{File: buf.Bytes(), Filename: fn}, nil
}

func formatUintIfNonZero(v uint64) string {
	if v == 0 {
		return ""
	}
	return strconv.FormatUint(v, 10)
}
