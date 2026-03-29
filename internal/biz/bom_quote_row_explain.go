package biz

import (
	"context"
	"fmt"
	"strings"
)

// QuoteRowMatchExplain 单行报价相对 BOM 在型号/封装/厂牌上的说明；PassesBomFilters 与 lineMatchRowPasses 一致。
type QuoteRowMatchExplain struct {
	RowIndex           int
	ModelOK            bool
	ModelReason        string
	PackageOK          bool
	PackageReason      string
	ManufacturerOK     bool
	ManufacturerReason string
	PassesBomFilters   bool
	Summary            string
}

// ExplainQuoteRowsForBOMLine 对缓存中每条报价行给出与配单筛选一致的中文说明。alias 可为 nil（厂牌解析视为未命中）。
func ExplainQuoteRowsForBOMLine(ctx context.Context, bomMpn, bomPackage, bomMfr string, rows []AgentQuoteRow, alias AliasLookup) ([]QuoteRowMatchExplain, error) {
	bomMfrTrim := strings.TrimSpace(bomMfr)
	bomPkgTrim := strings.TrimSpace(bomPackage)
	bomMpnNorm := NormalizeMPNForBOMSearch(bomMpn)

	var bomCanonID string
	var bomCanonHit bool
	if bomMfrTrim != "" {
		id, hit, err := ResolveManufacturerCanonical(ctx, bomMfr, alias)
		if err != nil {
			return nil, err
		}
		bomCanonID, bomCanonHit = id, hit
	}
	bomMfrRequired := bomMfrTrim != ""

	out := make([]QuoteRowMatchExplain, 0, len(rows))
	for i := range rows {
		row := rows[i]
		ex := QuoteRowMatchExplain{RowIndex: i}

		// 型号
		qm := strings.TrimSpace(row.Model)
		if qm == "" {
			ex.ModelOK = false
			ex.ModelReason = "报价型号为空"
		} else {
			qNorm := NormalizeMPNForBOMSearch(row.Model)
			if bomMpnNorm != qNorm {
				ex.ModelOK = false
				ex.ModelReason = fmt.Sprintf("归一化型号与需求不一致（需求键 %q，报价键 %q）", bomMpnNorm, qNorm)
			} else {
				ex.ModelOK = true
				ex.ModelReason = "归一化型号与需求一致"
			}
		}

		// 封装
		if bomPkgTrim == "" {
			ex.PackageOK = true
			ex.PackageReason = "需求未填封装，不校验"
		} else if qm == "" {
			ex.PackageOK = false
			ex.PackageReason = "报价型号为空，跳过封装比对"
		} else {
			if NormalizeMfrString(row.Package) != NormalizeMfrString(bomPackage) {
				ex.PackageOK = false
				ex.PackageReason = fmt.Sprintf("归一化封装与需求不一致（需求 %q，报价 %q）", NormalizeMfrString(bomPackage), NormalizeMfrString(row.Package))
			} else {
				ex.PackageOK = true
				ex.PackageReason = "归一化封装与需求一致"
			}
		}

		// 厂牌（严格：与 PickBestQuoteForLine 一致）
		if !bomMfrRequired {
			ex.ManufacturerOK = true
			ex.ManufacturerReason = "需求未填厂牌，不校验"
		} else if !bomCanonHit {
			ex.ManufacturerOK = false
			if alias == nil {
				ex.ManufacturerReason = "厂牌别名服务不可用，无法解析需求厂牌"
			} else {
				ex.ManufacturerReason = "需求厂牌未在别名表命中，严格模式下无法配单"
			}
		} else if strings.TrimSpace(row.Manufacturer) == "" {
			ex.ManufacturerOK = false
			ex.ManufacturerReason = "报价厂牌为空"
		} else {
			qCanon, qHit, err := ResolveManufacturerCanonical(ctx, row.Manufacturer, alias)
			if err != nil {
				return nil, err
			}
			if !qHit {
				ex.ManufacturerOK = false
				ex.ManufacturerReason = "报价厂牌未在别名表命中"
			} else if qCanon != bomCanonID {
				ex.ManufacturerOK = false
				ex.ManufacturerReason = fmt.Sprintf("规范厂牌与需求不一致（需求 canonical=%s，报价 canonical=%s）", bomCanonID, qCanon)
			} else {
				ex.ManufacturerOK = true
				ex.ManufacturerReason = "规范厂牌与需求一致"
			}
		}

		rowPasses, err := lineMatchRowPasses(ctx, LineMatchInput{
			BomMpn:     bomMpn,
			BomPackage: bomPackage,
			BomMfr:     bomMfr,
		}, bomCanonID, bomMfrRequired, row, alias)
		if err != nil {
			return nil, err
		}
		ex.PassesBomFilters = rowPasses

		ex.Summary = summarizeQuoteRowExplain(ex)
		out = append(out, ex)
	}
	return out, nil
}

func summarizeQuoteRowExplain(ex QuoteRowMatchExplain) string {
	if !ex.ModelOK {
		return "未通过：型号"
	}
	if !ex.PackageOK {
		return "未通过：封装"
	}
	if !ex.ManufacturerOK {
		return "未通过：厂牌"
	}
	return "通过 BOM 筛选（型号/封装/厂牌）；是否中选还取决于可解析价格、汇率与排序策略"
}
