package data

import (
	"context"
	"strings"

	"caichip/internal/biz"
)

// CachedBomManufacturerAliasRepo 厂牌别名读穿 + 写后失效。
type CachedBomManufacturerAliasRepo struct {
	inner *BomManufacturerAliasRepo
	kv    *InprocKV
}

// NewCachedBomManufacturerAliasRepo ...
func NewCachedBomManufacturerAliasRepo(inner *BomManufacturerAliasRepo, kv *InprocKV) *CachedBomManufacturerAliasRepo {
	if inner == nil {
		inner = &BomManufacturerAliasRepo{}
	}
	if kv == nil {
		kv = NewInprocKV()
	}
	return &CachedBomManufacturerAliasRepo{inner: inner, kv: kv}
}

func cloneManufacturerCanonicalRows(in []biz.ManufacturerCanonicalDisplay) []biz.ManufacturerCanonicalDisplay {
	out := make([]biz.ManufacturerCanonicalDisplay, len(in))
	copy(out, in)
	return out
}

func (r *CachedBomManufacturerAliasRepo) CanonicalID(ctx context.Context, aliasNorm string) (string, bool, error) {
	norm := strings.TrimSpace(aliasNorm)
	if norm == "" {
		return r.inner.CanonicalID(ctx, aliasNorm)
	}
	key := KeyMfrAliasNorm(norm)
	if v, ok := r.kv.Get(key); ok {
		if p, ok := v.(*mfrCanonCacheEntry); ok {
			return p.id, p.hit, nil
		}
	}
	id, hit, err := r.inner.CanonicalID(ctx, aliasNorm)
	if err != nil {
		return "", false, err
	}
	r.kv.Set(key, &mfrCanonCacheEntry{id: id, hit: hit})
	return id, hit, nil
}

type mfrCanonCacheEntry struct {
	id  string
	hit bool
}

func (r *CachedBomManufacturerAliasRepo) DBOk() bool {
	return r.inner.DBOk()
}

func (r *CachedBomManufacturerAliasRepo) ListDistinctCanonicals(ctx context.Context, limit int) ([]biz.ManufacturerCanonicalDisplay, error) {
	key := KeyMfrAliasCanonicalsList(limit)
	if v, ok := r.kv.Get(key); ok {
		if rows, ok := v.([]biz.ManufacturerCanonicalDisplay); ok {
			return cloneManufacturerCanonicalRows(rows), nil
		}
	}
	rows, err := r.inner.ListDistinctCanonicals(ctx, limit)
	if err != nil {
		return nil, err
	}
	r.kv.Set(key, cloneManufacturerCanonicalRows(rows))
	return cloneManufacturerCanonicalRows(rows), nil
}

func (r *CachedBomManufacturerAliasRepo) CreateRow(ctx context.Context, canonicalID, displayName, alias, aliasNorm string) error {
	if err := r.inner.CreateRow(ctx, canonicalID, displayName, alias, aliasNorm); err != nil {
		return err
	}
	norm := strings.TrimSpace(aliasNorm)
	if norm != "" {
		r.kv.Delete(KeyMfrAliasNorm(norm))
	}
	r.kv.DeletePrefix(prefixMfrAliasCanon)
	return nil
}

var _ biz.BomManufacturerAliasRepo = (*CachedBomManufacturerAliasRepo)(nil)
var _ biz.AliasLookup = (*CachedBomManufacturerAliasRepo)(nil)
