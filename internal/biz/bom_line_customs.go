package biz

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

var reHSCodeTS10 = regexp.MustCompile(`^\d{10}$`)

func isValidHSCodeTS(s string) bool {
	return reHSCodeTS10.MatchString(strings.TrimSpace(s))
}

func bomLineMfrString(mfr *string) string {
	if mfr == nil {
		return ""
	}
	return strings.TrimSpace(*mfr)
}

// pickTaxRateRow 选取与请求 code_ts 一致的首条税率记录（design §4.3）。
func pickTaxRateRow(codeTS string, res *TaxRateFetchResult) (*TaxRateAPIItemRow, bool) {
	if res == nil {
		return nil, false
	}
	want := strings.TrimSpace(codeTS)
	for i := range res.Items {
		if strings.TrimSpace(res.Items[i].CodeTS) == want {
			return &res.Items[i], true
		}
	}
	return nil, false
}

// FillBomLineCustoms 为配单行填充 HS / 商检 / 关税日缓存结果（不含 Proto 写入）。
func FillBomLineCustoms(
	ctx context.Context,
	lines []BomLineCustomsLine,
	mappingRepo HsModelMappingRepo,
	itemRepo HsItemReadRepo,
	dailyRepo HsTaxRateDailyRepo,
	taxAPI TaxRateAPIFetcher,
	now func() time.Time,
) ([]BomLineCustomsOut, error) {
	if len(lines) == 0 {
		return nil, nil
	}
	if now == nil {
		now = time.Now
	}
	if mappingRepo == nil || !mappingRepo.DBOk() ||
		itemRepo == nil || !itemRepo.DBOk() ||
		dailyRepo == nil || !dailyRepo.DBOk() ||
		taxAPI == nil {
		return nil, fmt.Errorf("bom_line_customs: dependencies not ready")
	}

	out := make([]BomLineCustomsOut, len(lines))
	codeByLine := make([]string, len(lines))

	for i := range lines {
		out[i].LineNo = lines[i].LineNo
		model := strings.TrimSpace(lines[i].Mpn)
		mfr := bomLineMfrString(lines[i].Mfr)
		rec, err := mappingRepo.GetConfirmedByModelManufacturer(ctx, model, mfr)
		if err != nil {
			return nil, err
		}
		if rec == nil {
			out[i].HsCodeStatus = HsCodeStatusNotMapped
			continue
		}
		code := strings.TrimSpace(rec.CodeTS)
		if !isValidHSCodeTS(code) {
			out[i].HsCodeStatus = HsCodeStatusCodeInvalid
			continue
		}
		out[i].HsCodeStatus = HsCodeStatusFound
		out[i].CodeTS = code
		codeByLine[i] = code
	}

	var uniq []string
	seen := make(map[string]struct{})
	for _, c := range codeByLine {
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		uniq = append(uniq, c)
	}

	itemMap, err := itemRepo.MapByCodeTS(ctx, uniq)
	if err != nil {
		return nil, err
	}
	for i := range lines {
		if out[i].HsCodeStatus != HsCodeStatusFound {
			continue
		}
		code := codeByLine[i]
		it, ok := itemMap[code]
		if !ok || it == nil {
			out[i].CustomsErrors = append(out[i].CustomsErrors, CustomsErrHSItemMissing)
			continue
		}
		out[i].ControlMark = strings.TrimSpace(it.ControlMark)
	}

	bizDate := truncateLocalDate(now())
	dailyMap, err := dailyRepo.GetManyByBizDate(ctx, bizDate, uniq)
	if err != nil {
		return nil, err
	}

	var need []string
	for _, c := range uniq {
		if _, ok := dailyMap[c]; !ok {
			need = append(need, c)
		}
	}

	var taxMu sync.Mutex
	taxErr := make(map[string]error)
	fetched := make(map[string]*HsTaxRateDailyRecord)

	if len(need) > 0 {
		eg, gctx := errgroup.WithContext(ctx)
		eg.SetLimit(4)
		for _, code := range need {
			code := code
			eg.Go(func() error {
				res, ferr := taxAPI.FetchByCodeTS(gctx, code, 10)
				if ferr != nil {
					taxMu.Lock()
					taxErr[code] = ferr
					taxMu.Unlock()
					return nil
				}
				row, ok := pickTaxRateRow(code, res)
				if !ok || row == nil {
					taxMu.Lock()
					taxErr[code] = fmt.Errorf("tax_rate: no row for code_ts")
					taxMu.Unlock()
					return nil
				}
				rec := &HsTaxRateDailyRecord{
					CodeTS:          code,
					BizDate:         bizDate,
					GName:           row.GName,
					ImpDiscountRate: row.ImpDiscountRate,
					ImpTempRate:     row.ImpTempRate,
					ImpOrdinaryRate: row.ImpOrdinaryRate,
				}
				if uerr := dailyRepo.Upsert(gctx, rec); uerr != nil {
					refreshed, rerr := dailyRepo.GetManyByBizDate(gctx, bizDate, []string{code})
					if rerr == nil && refreshed != nil {
						if rr, ok2 := refreshed[code]; ok2 && rr != nil {
							taxMu.Lock()
							fetched[code] = rr
							taxMu.Unlock()
							return nil
						}
					}
					taxMu.Lock()
					taxErr[code] = uerr
					taxMu.Unlock()
					return nil
				}
				taxMu.Lock()
				fetched[code] = rec
				taxMu.Unlock()
				return nil
			})
		}
		_ = eg.Wait()
		for k, v := range fetched {
			dailyMap[k] = v
		}
		ref, rerr := dailyRepo.GetManyByBizDate(ctx, bizDate, uniq)
		if rerr == nil && ref != nil {
			for k, v := range ref {
				dailyMap[k] = v
			}
		}
	}

	for i := range lines {
		if out[i].HsCodeStatus != HsCodeStatusFound {
			continue
		}
		code := codeByLine[i]
		if rec, ok := dailyMap[code]; ok && rec != nil {
			out[i].ImportTaxGName = rec.GName
			out[i].ImportTaxImpOrdinaryRate = rec.ImpOrdinaryRate
			out[i].ImportTaxImpDiscountRate = rec.ImpDiscountRate
			out[i].ImportTaxImpTempRate = rec.ImpTempRate
			continue
		}
		if e, ok := taxErr[code]; ok && e != nil {
			if strings.Contains(strings.ToLower(e.Error()), "no row") {
				out[i].CustomsErrors = append(out[i].CustomsErrors, CustomsErrTaxNoRow)
			} else {
				out[i].CustomsErrors = append(out[i].CustomsErrors, CustomsErrTaxAPI)
			}
		}
	}

	return out, nil
}

func truncateLocalDate(t time.Time) time.Time {
	t = t.In(time.Local)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}
