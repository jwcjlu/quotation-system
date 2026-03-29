package biz

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ErrFXRateNotFound 无可用汇率（含直连与反向行均缺失）；调用方可跳过该候选报价。
var ErrFXRateNotFound = errors.New("bom fx: rate not found")

// FXDateSource 审计：汇率查表所用日历日的来源（见设计 §1.8）。
const (
	FXDateSourceBizDate    = "biz_date"
	FXDateSourceRequestDay = "request_day"
)

// FXMeta 单次换算的汇率与审计字段。
type FXMeta struct {
	Rate           float64
	TableVersion   string
	Source         string
	FxDate         time.Time
	FxDateSource   string // biz_date | request_day
}

// FXRateLookup 按业务日查「1 from = rate × to」的汇率；data 层实现。
type FXRateLookup interface {
	Rate(ctx context.Context, from, to string, date time.Time) (rate float64, tableVersion, source string, ok bool)
}

// ToBaseCCY 将原币单价换算为 base_ccy。
// 汇率语义与表 t_bom_fx_rate 一致：1 单位 from = rate 单位 base。
// 查表日优先 biz_date（非零）；否则用 requestDay，并在 FXMeta.FxDateSource 中区分。
//
// 交叉币种（V1）：先查 from→base；若无行再查 base→from，若存在则用倒数 1/rate
//（例如仅落库 USD→CNY 时，CNY→USD 可走反向行）。若 rate 为 0 则视为不可用。
func ToBaseCCY(ctx context.Context, price float64, fromCCY, baseCCY string, bizDate, requestDay time.Time, lookup FXRateLookup) (base float64, meta FXMeta, err error) {
	from := normFXCCY(fromCCY)
	baseCcy := normFXCCY(baseCCY)
	if from == "" || baseCcy == "" {
		return 0, FXMeta{}, errors.New("bom fx: from_ccy and base_ccy required")
	}

	fxDate, fxSrc := pickFXLookupDate(bizDate, requestDay)
	fxDate = truncateFXDateUTC(fxDate)

	if from == baseCcy {
		return price, FXMeta{
			Rate:         1,
			FxDate:       fxDate,
			FxDateSource: fxSrc,
		}, nil
	}
	if lookup == nil {
		return 0, FXMeta{}, ErrFXRateNotFound
	}

	rate, tv, src, ok := lookup.Rate(ctx, from, baseCcy, fxDate)
	if ok && rate != 0 {
		return price * rate, FXMeta{
			Rate:         rate,
			TableVersion: tv,
			Source:       src,
			FxDate:       fxDate,
			FxDateSource: fxSrc,
		}, nil
	}

	// 反向：表中存的是 1 base = inv × from，故 1 from = (1/inv) × base。
	inv, tv2, src2, ok2 := lookup.Rate(ctx, baseCcy, from, fxDate)
	if !ok2 || inv == 0 {
		return 0, FXMeta{}, ErrFXRateNotFound
	}
	eff := 1 / inv
	return price * eff, FXMeta{
		Rate:         eff,
		TableVersion: tv2,
		Source:       src2,
		FxDate:       fxDate,
		FxDateSource: fxSrc,
	}, nil
}

func normFXCCY(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

func pickFXLookupDate(bizDate, requestDay time.Time) (time.Time, string) {
	if !bizDate.IsZero() {
		return bizDate, FXDateSourceBizDate
	}
	return requestDay, FXDateSourceRequestDay
}

func truncateFXDateUTC(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	y, m, d := t.In(time.UTC).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
