package data

import (
	"context"
	"errors"
	"strings"
	"time"

	"caichip/internal/biz"

	"golang.org/x/sync/singleflight"
)

// 生产默认 TTL；测试可在同 package 内临时改写以加速过期验证。
var (
	mfrAliasCanonPosTTL = 15 * time.Minute
	mfrAliasCanonNegTTL = 2 * time.Minute
)

// CachedBomManufacturerAliasRepo 厂牌别名读穿 + 写后失效。
type CachedBomManufacturerAliasRepo struct {
	inner *BomManufacturerAliasRepo
	alias biz.AliasLookup
	kv    *InprocKV
	sf    singleflight.Group
}

// NewCachedBomManufacturerAliasRepo ...
func NewCachedBomManufacturerAliasRepo(inner *BomManufacturerAliasRepo, kv *InprocKV) *CachedBomManufacturerAliasRepo {
	if inner == nil {
		inner = &BomManufacturerAliasRepo{}
	}
	if kv == nil {
		kv = NewInprocKV()
	}
	return &CachedBomManufacturerAliasRepo{inner: inner, alias: inner, kv: kv}
}

func cloneManufacturerCanonicalRows(in []biz.ManufacturerCanonicalDisplay) []biz.ManufacturerCanonicalDisplay {
	out := make([]biz.ManufacturerCanonicalDisplay, len(in))
	copy(out, in)
	return out
}

func (r *CachedBomManufacturerAliasRepo) CanonicalID(ctx context.Context, aliasNorm string) (string, bool, error) {
	norm := strings.TrimSpace(aliasNorm)
	if norm == "" {
		return r.alias.CanonicalID(ctx, aliasNorm)
	}
	key := KeyMfrAliasNorm(norm)
	if v, ok := r.kv.Get(key); ok {
		if p, ok := v.(*mfrCanonCacheEntry); ok {
			if p.expiresAt.IsZero() || time.Now().After(p.expiresAt) {
				r.kv.Delete(key)
			} else {
				return p.id, p.hit, nil
			}
		}
	}
	v, err, _ := r.sf.Do(key, func() (any, error) {
		id, hit, ierr := r.alias.CanonicalID(ctx, aliasNorm)
		if ierr != nil {
			return nil, ierr
		}
		ttl := mfrAliasCanonNegTTL
		if hit {
			ttl = mfrAliasCanonPosTTL
		}
		entry := &mfrCanonCacheEntry{id: id, hit: hit, expiresAt: time.Now().Add(ttl)}
		r.kv.Set(key, entry)
		return entry, nil
	})
	if err != nil {
		return "", false, err
	}
	p := v.(*mfrCanonCacheEntry)
	return p.id, p.hit, nil
}

// CanonicalIDsByNormKeys 合并缓存未命中键后，对剩余键一次穿透查询并回填 TTL 缓存。
func (r *CachedBomManufacturerAliasRepo) CanonicalIDsByNormKeys(ctx context.Context, aliasNormKeys []string) (map[string]string, error) {
	if r == nil || r.inner == nil {
		return map[string]string{}, nil
	}
	uniq := dedupeNonEmptyAliasNormKeys(aliasNormKeys)
	if len(uniq) == 0 {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(uniq))
	fetchKeys := make([]string, 0, len(uniq))
	for _, k := range uniq {
		key := KeyMfrAliasNorm(k)
		if v, ok := r.kv.Get(key); ok {
			if p, ok := v.(*mfrCanonCacheEntry); ok {
				expired := p.expiresAt.IsZero() || time.Now().After(p.expiresAt)
				if !expired {
					if p.hit {
						out[k] = p.id
					}
					continue
				}
				r.kv.Delete(key)
			}
		}
		fetchKeys = append(fetchKeys, k)
	}
	if len(fetchKeys) == 0 {
		return out, nil
	}
	dbMap, err := r.inner.CanonicalIDsByNormKeys(ctx, fetchKeys)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for _, k := range fetchKeys {
		canon, ok := dbMap[k]
		canon = strings.TrimSpace(canon)
		hit := ok && canon != ""
		var id string
		if hit {
			id = canon
			out[k] = id
		}
		ttl := mfrAliasCanonNegTTL
		if hit {
			ttl = mfrAliasCanonPosTTL
		}
		r.kv.Set(KeyMfrAliasNorm(k), &mfrCanonCacheEntry{id: id, hit: hit, expiresAt: now.Add(ttl)})
	}
	return out, nil
}

type mfrCanonCacheEntry struct {
	id        string
	hit       bool
	expiresAt time.Time
}

func (r *CachedBomManufacturerAliasRepo) DBOk() bool {
	if r == nil || r.inner == nil {
		return false
	}
	return r.inner.DBOk()
}

func (r *CachedBomManufacturerAliasRepo) ListDistinctCanonicals(ctx context.Context, limit int) ([]biz.ManufacturerCanonicalDisplay, error) {
	if r == nil || r.inner == nil {
		return nil, errors.New("bom manufacturer alias: database not configured")
	}
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
	if r == nil || r.inner == nil {
		return errors.New("bom manufacturer alias: database not configured")
	}
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
