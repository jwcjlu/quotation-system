package biz

import (
	"context"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// NormalizeMfrString 厂牌字符串规范化：trim、NFKC（全角拉丁等 → 半角兼容形）、再对 Unicode 字母做大小写折叠。
// 与 NormalizeMPNForBOMSearch 一致之处：均使用 strings.ToUpper 做字母大小写统一；厂牌路径额外做 NFKC 以满足设计 §2 全半角约定
// （MPN 搜索键当前仅 trim + ToUpper，二者在仅含半角字母数字时行为一致）。
func NormalizeMfrString(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = norm.NFKC.String(s)
	return strings.ToUpper(s)
}

// AliasLookup 厂牌别名表查询：alias_norm 与 NormalizeMfrString 输出一致时命中。
// ok=true 且 err=nil 表示命中；ok=false 且 err=nil 表示无行；err!=nil 表示数据库等基础设施错误。
type AliasLookup interface {
	CanonicalID(ctx context.Context, aliasNorm string) (canonicalID string, ok bool, err error)
}

// ResolveManufacturerCanonical 将 BOM/报价原始厂牌串解析为规范 ID（严格模式）。
// 若 raw 在去空白后为空：返回 ("", false, nil)，表示 §2.5 不施加厂牌约束（与别名未命中同为 hit=false，调用方需结合 TrimSpace(raw) 区分）。
// 若非空：经 NormalizeMfrString 后查表；未命中则 ("", false, nil)（§2.3 严格策略）。
// err!=nil 表示别名表查询失败（须与未命中区分）。
func ResolveManufacturerCanonical(ctx context.Context, raw string, lookup AliasLookup) (canonicalID string, hit bool, err error) {
	if strings.TrimSpace(raw) == "" {
		return "", false, nil
	}
	if lookup == nil {
		return "", false, nil
	}
	normKey := NormalizeMfrString(raw)
	if normKey == "" {
		return "", false, nil
	}
	id, ok, lerr := lookup.CanonicalID(ctx, normKey)
	if lerr != nil {
		return "", false, lerr
	}
	if !ok {
		return "", false, nil
	}
	return id, true, nil
}
