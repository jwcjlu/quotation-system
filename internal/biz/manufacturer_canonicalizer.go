package biz

import (
	"context"
	"strings"
)

// ManufacturerCanonicalizer 将原始厂牌字符串解析为 canonical_id。
// 规则：
// 1) 空白输入：未命中（不施加厂牌约束）；
// 2) 非空输入：先 NormalizeMfrString，再通过 AliasLookup 点查；
// 3) lookup 返回错误时，向上返回错误，调用方需与未命中区分。
type ManufacturerCanonicalizer struct {
	lookup AliasLookup
}

func NewManufacturerCanonicalizer(lookup AliasLookup) *ManufacturerCanonicalizer {
	return &ManufacturerCanonicalizer{lookup: lookup}
}

func (c *ManufacturerCanonicalizer) Resolve(ctx context.Context, raw string) (canonicalID string, hit bool, err error) {
	if strings.TrimSpace(raw) == "" {
		return "", false, nil
	}
	// lookup 未注入时视为“无别名能力”，按未命中处理（与历史行为一致，避免把可选依赖当成硬失败）。
	if c == nil || c.lookup == nil {
		return "", false, nil
	}

	normKey := NormalizeMfrString(raw)
	if normKey == "" {
		return "", false, nil
	}

	id, ok, lerr := c.lookup.CanonicalID(ctx, normKey)
	if lerr != nil {
		return "", false, lerr
	}
	if !ok {
		return "", false, nil
	}
	return id, true, nil
}

// ResolveManufacturerCanonical 为现有调用方提供兼容入口。
func ResolveManufacturerCanonical(ctx context.Context, raw string, lookup AliasLookup) (canonicalID string, hit bool, err error) {
	return NewManufacturerCanonicalizer(lookup).Resolve(ctx, raw)
}
