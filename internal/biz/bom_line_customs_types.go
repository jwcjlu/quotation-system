package biz

import "time"

// HS 行级状态（与 design §2.3 一致）。
const (
	HsCodeStatusFound       = "hs_found"
	HsCodeStatusNotMapped   = "hs_not_mapped"
	HsCodeStatusCodeInvalid = "hs_code_invalid"
)

// 分项错误码（写入 MatchItem.hs_customs_error，分号拼接）。
const (
	CustomsErrHSItemMissing = "hs_item_missing"
	CustomsErrTaxAPI        = "tax_api_failed"
	CustomsErrTaxNoRow      = "tax_no_matching_row"
)

// BomLineCustomsLine 配单海关扩展的输入行（无 DB 类型依赖）。
type BomLineCustomsLine struct {
	LineNo int
	Mpn    string
	Mfr    *string
}

// BomLineCustomsOut 单行输出，供 service 写入 Proto。
type BomLineCustomsOut struct {
	LineNo                   int
	HsCodeStatus             string
	CodeTS                   string
	ControlMark              string
	ImportTaxGName           string
	ImportTaxImpOrdinaryRate string
	ImportTaxImpDiscountRate string
	ImportTaxImpTempRate     string
	CustomsErrors            []string
}

// HsTaxRateDailyRecord 关税日缓存一行（与 t_hs_tax_rate_daily 列语义一致）。
type HsTaxRateDailyRecord struct {
	CodeTS          string
	BizDate         time.Time
	GName           string
	ImpDiscountRate string
	ImpTempRate     string
	ImpOrdinaryRate string
}

// TaxRateAPIItemRow 税率接口 data.data[] 单条（biz 自有类型）。
type TaxRateAPIItemRow struct {
	CodeTS          string
	GName           string
	ImpDiscountRate string
	ImpTempRate     string
	ImpOrdinaryRate string
}

// TaxRateFetchResult 税率接口解析结果（biz 抽象）。
type TaxRateFetchResult struct {
	Items []TaxRateAPIItemRow
}
