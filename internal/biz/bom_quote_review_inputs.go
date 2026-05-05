package biz

import (
	"context"
	"errors"
	"strings"
	"time"
)

// QuoteReviewPriceDeps 从报价只读行提取可比价（与配单 PickBestQuoteForLine 口径对齐：Extract + ToBase + Quantize）。
type QuoteReviewPriceDeps struct {
	BomQty           int
	BaseCCY          string
	RoundingMode     string
	ParseTierStrings bool
	BizDate          time.Time
	RequestDay       time.Time
	FX               FXRateLookup
}

// BuildQuoteReviewRowInputs 将会话行下的报价只读行转为规则 B 输入（首期 E：全量返回行 InE=true，由调用方传入）。
func BuildQuoteReviewRowInputs(ctx context.Context, readRows []BomQuoteItemReadRow, dep QuoteReviewPriceDeps) ([]QuoteReviewRowInput, error) {
	if dep.BomQty <= 0 {
		dep.BomQty = 1
	}
	if dep.FX == nil {
		return nil, errors.New("bom quote review: fx lookup required")
	}
	out := make([]QuoteReviewRowInput, 0, len(readRows))
	for i := range readRows {
		r := readRows[i]
		pid := NormalizePlatformID(r.PlatformID)
		defaultCcy := DefaultQuoteCCY(pid)
		moq := parseMoqDigits(r.MOQ)
		qIn := QuotePriceInput{
			UnitPrice:     0,
			MainlandPrice: r.MainlandPrice,
			HkPrice:       r.HKPrice,
			PriceTiers:    r.PriceTiers,
			Moq:           moq,
			Stock:         r.Stock,
		}
		cp := ExtractCompareUnitPrice(qIn, pid, dep.BomQty, dep.ParseTierStrings, defaultCcy)
		var pricePtr *float64
		if cp.Ok {
			base, _, err := ToBaseCCY(ctx, cp.Price, cp.Ccy, dep.BaseCCY, dep.BizDate, dep.RequestDay, dep.FX)
			if err != nil {
				if errors.Is(err, ErrFXRateNotFound) {
					pricePtr = nil
				} else {
					return nil, err
				}
			} else {
				// 规则 B 排序使用 base_ccy 单价（与配单同口径换算）；不在此重复 Quantize，避免与 float64 排序键类型纠缠。
				pricePtr = &base
			}
		}
		out = append(out, QuoteReviewRowInput{
			InE:          true,
			Status:       NormalizeMfrReviewStatus(r.ManufacturerReviewStatus),
			ComparePrice: pricePtr,
			UpdatedAt:    time.Time{},
			ItemID:       r.ItemID,
		})
	}
	return out, nil
}

// NormalizeMfrReviewStatus 归一化厂牌评审状态（与落库枚举对齐）。
func NormalizeMfrReviewStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case MfrReviewAccepted:
		return MfrReviewAccepted
	case MfrReviewRejected:
		return MfrReviewRejected
	default:
		return MfrReviewPending
	}
}
