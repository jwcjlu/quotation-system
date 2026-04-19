package data

import (
	"context"
	"errors"
	"strings"

	"caichip/internal/biz"
)

// TaxRateAPIBizAdapter 将 HsTaxRateAPIRepo 适配为 biz.TaxRateAPIFetcher。
type TaxRateAPIBizAdapter struct {
	inner *HsTaxRateAPIRepo
}

func NewTaxRateAPIBizAdapter(inner *HsTaxRateAPIRepo) *TaxRateAPIBizAdapter {
	return &TaxRateAPIBizAdapter{inner: inner}
}

func (a *TaxRateAPIBizAdapter) FetchByCodeTS(ctx context.Context, codeTS string, pageSize int) (*biz.TaxRateFetchResult, error) {
	if a == nil || a.inner == nil {
		return nil, errors.New("tax_rate_api_biz_adapter: nil inner repo")
	}
	got, err := a.inner.FetchByCodeTS(ctx, codeTS, pageSize)
	if err != nil {
		return nil, err
	}
	if got == nil {
		return &biz.TaxRateFetchResult{}, nil
	}
	out := make([]biz.TaxRateAPIItemRow, 0, len(got.Items))
	for i := range got.Items {
		it := got.Items[i]
		out = append(out, biz.TaxRateAPIItemRow{
			CodeTS:          strings.TrimSpace(it.CodeTS),
			GName:           strings.TrimSpace(it.GName),
			ImpDiscountRate: strings.TrimSpace(it.ImpDiscountRate),
			ImpTempRate:     strings.TrimSpace(it.ImpTempRate),
			ImpOrdinaryRate: strings.TrimSpace(it.ImpOrdinaryRate),
		})
	}
	return &biz.TaxRateFetchResult{Items: out}, nil
}

var _ biz.TaxRateAPIFetcher = (*TaxRateAPIBizAdapter)(nil)
