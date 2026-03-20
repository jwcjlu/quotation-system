package biz

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// MatchStrategy 配单策略
const (
	StrategyPriceFirst    = "price_first"
	StrategyStockFirst    = "stock_first"
	StrategyLeadtimeFirst = "leadtime_first"
	StrategyComprehensive = "comprehensive"
)

// MatchUseCase 配单业务用例
type MatchUseCase struct {
	bomRepo    BOMRepo
	searchRepo SearchRepo
}

// SearchRepo 搜索仓储（获取报价数据与配单结果）
type SearchRepo interface {
	GetQuotesByBOM(ctx context.Context, bomID string) ([]*ItemQuotes, error)
	SaveQuotes(ctx context.Context, bomID string, quotes []*ItemQuotes) error
	SaveMatchResult(ctx context.Context, bomID string, items []*MatchItem) error
	GetMatchResult(ctx context.Context, bomID string) ([]*MatchItem, error)
}

// NewMatchUseCase 创建配单用例
func NewMatchUseCase(bomRepo BOMRepo, searchRepo SearchRepo) *MatchUseCase {
	return &MatchUseCase{
		bomRepo:    bomRepo,
		searchRepo: searchRepo,
	}
}

// AutoMatch 自动配单
func (uc *MatchUseCase) AutoMatch(ctx context.Context, bomID string, strategy string) ([]*MatchItem, float64, error) {
	bom, err := uc.bomRepo.GetBOM(ctx, bomID)
	if err != nil {
		return nil, 0, err
	}
	if bom == nil {
		return nil, 0, ErrBOMNotFound
	}

	itemQuotes, err := uc.searchRepo.GetQuotesByBOM(ctx, bomID)
	if err != nil {
		return nil, 0, err
	}

	quotesByModel := make(map[string]*ItemQuotes)
	for _, iq := range itemQuotes {
		quotesByModel[iq.Model] = iq
	}

	var totalAmount float64
	items := make([]*MatchItem, 0, len(bom.Items))

	for idx, item := range bom.Items {
		matchItem := &MatchItem{
			Index:              idx + 1,
			Model:              item.Model,
			Quantity:           item.Quantity,
			DemandManufacturer: item.Manufacturer,
			DemandPackage:      item.Package,
		}

		iq := quotesByModel[item.Model]
		if iq == nil || len(iq.Quotes) == 0 {
			matchItem.MatchStatus = "no_match"
			items = append(items, matchItem)
			continue
		}

		// 按型号/封装/厂牌完全匹配筛选后，再选型
		matched := filterFullyMatched(iq.Quotes, item)
		best := selectBest(matched, item.Quantity, strategy)
		if best != nil {
			matchItem.MatchedModel = best.MatchedModel
			matchItem.Manufacturer = best.Manufacturer
			matchItem.Platform = best.Platform
			matchItem.LeadTime = best.LeadTime
			matchItem.Stock = best.Stock
			matchItem.UnitPrice = best.UnitPrice
			matchItem.Subtotal = best.UnitPrice * float64(item.Quantity)
			matchItem.MatchStatus = "exact"
			totalAmount += matchItem.Subtotal
		} else {
			matchItem.MatchStatus = "pending"
		}

		matchItem.AllQuotes = iq.Quotes
		items = append(items, matchItem)
	}

	if err := uc.searchRepo.SaveMatchResult(ctx, bomID, items); err != nil {
		return items, totalAmount, nil
	}

	return items, totalAmount, nil
}

// filterFullyMatched 按型号、封装、厂牌完全匹配筛选
func filterFullyMatched(quotes []*Quote, item *BOMItem) []*Quote {
	if len(quotes) == 0 {
		return nil
	}
	var out []*Quote
	for _, q := range quotes {
		if !matchField(q.MatchedModel, item.Model) {
			continue
		}
		if !matchField(q.Manufacturer, item.Manufacturer) {
			continue
		}
		if !matchField(q.Package, item.Package) {
			continue
		}
		out = append(out, q)
	}
	return out
}

func matchField(qVal, itemVal string) bool {
	qVal = strings.TrimSpace(strings.ToUpper(qVal))
	itemVal = strings.TrimSpace(strings.ToUpper(itemVal))
	if itemVal == "" || itemVal == "N/A" {
		return true
	}
	if qVal == "" || qVal == "N/A" {
		return false
	}
	return qVal == itemVal
}

func selectBest(quotes []*Quote, quantity int, strategy string) *Quote {
	if len(quotes) == 0 {
		return nil
	}

	sorted := make([]*Quote, len(quotes))
	copy(sorted, quotes)

	switch strategy {
	case StrategyPriceFirst:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].UnitPrice < sorted[j].UnitPrice
		})
	case StrategyStockFirst:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Stock > sorted[j].Stock
		})
	case StrategyLeadtimeFirst:
		sort.Slice(sorted, func(i, j int) bool {
			return parseLeadTimeDays(sorted[i].LeadTime) < parseLeadTimeDays(sorted[j].LeadTime)
		})
	case StrategyComprehensive:
		sort.Slice(sorted, func(i, j int) bool {
			pi, pj := sorted[i], sorted[j]
			si := scoreQuote(pi, quantity)
			sj := scoreQuote(pj, quantity)
			return si > sj
		})
	default:
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].UnitPrice < sorted[j].UnitPrice
		})
	}

	return sorted[0]
}

func parseLeadTimeDays(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 999
	}
	if idx := strings.Index(s, "周"); idx >= 0 {
		n, _ := strconv.Atoi(strings.TrimSpace(s[:idx]))
		return n * 7
	}
	if idx := strings.Index(s, "工作日"); idx >= 0 {
		part := strings.TrimSpace(s[:idx])
		if idx2 := strings.Index(part, "-"); idx2 >= 0 {
			n, _ := strconv.Atoi(strings.TrimSpace(part[:idx2]))
			return n
		}
		n, _ := strconv.Atoi(part)
		return n
	}
	return 999
}

func scoreQuote(q *Quote, quantity int) float64 {
	priceScore := 100.0 / (1 + q.UnitPrice)
	stockScore := float64(q.Stock) / 1000
	if stockScore > 10 {
		stockScore = 10
	}
	leadTimeScore := 10.0 / float64(1+parseLeadTimeDays(q.LeadTime))
	return priceScore*0.5 + stockScore*0.3 + leadTimeScore*0.2
}
