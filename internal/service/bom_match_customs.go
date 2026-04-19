package service

import (
	"context"
	"strings"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/data"
)

// customsDepsOK：策略 A — 依赖齐全才填充 HS/商检/关税字段；否则跳过（不配单失败）。
func (s *BomService) customsDepsOK() bool {
	return s.hsMapping != nil && s.hsMapping.DBOk() &&
		s.hsItem != nil && s.hsItem.DBOk() &&
		s.hsTaxDaily != nil && s.hsTaxDaily.DBOk() &&
		s.hsTaxAPI != nil
}

func (s *BomService) attachCustomsToMatchItems(ctx context.Context, lines []data.BomSessionLine, items []*v1.MatchItem) {
	if !s.customsDepsOK() || len(lines) == 0 || len(items) == 0 {
		return
	}
	if len(lines) != len(items) {
		s.log.Debugf("bom customs skip: lines=%d items=%d", len(lines), len(items))
		return
	}
	blines := make([]biz.BomLineCustomsLine, len(lines))
	for i := range lines {
		blines[i] = biz.BomLineCustomsLine{
			LineNo: lines[i].LineNo,
			Mpn:    lines[i].Mpn,
			Mfr:    lines[i].Mfr,
		}
	}
	outs, err := biz.FillBomLineCustoms(ctx, blines, s.hsMapping, s.hsItem, s.hsTaxDaily, s.hsTaxAPI, nil)
	if err != nil {
		s.log.Debugf("bom customs: %v", err)
		return
	}
	if len(outs) != len(items) {
		s.log.Debugf("bom customs skip: outs=%d items=%d", len(outs), len(items))
		return
	}
	for i := range items {
		o := outs[i]
		if int(items[i].Index) != o.LineNo {
			s.log.Warnf("bom customs line align: item.index=%d customs.line_no=%d", items[i].Index, o.LineNo)
		}
		items[i].HsCodeStatus = o.HsCodeStatus
		items[i].CodeTs = o.CodeTS
		items[i].ControlMark = o.ControlMark
		items[i].ImportTaxGName = o.ImportTaxGName
		items[i].ImportTaxImpOrdinaryRate = o.ImportTaxImpOrdinaryRate
		items[i].ImportTaxImpDiscountRate = o.ImportTaxImpDiscountRate
		items[i].ImportTaxImpTempRate = o.ImportTaxImpTempRate
		if len(o.CustomsErrors) > 0 {
			items[i].HsCustomsError = strings.Join(o.CustomsErrors, ";")
		}
	}
}
