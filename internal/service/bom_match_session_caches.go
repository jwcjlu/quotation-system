package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"caichip/internal/biz"
	"caichip/internal/data"
)

// quoteCachePairKey 与 data.LoadQuoteCachesForKeys 返回的 map 键一致。
func quoteCachePairKey(mpnNorm, platformID string) string {
	return mpnNorm + "\x00" + platformID
}

// dedupeQuoteCachePairs 构建 (merge_mpn × platform) 去重列表，供批量拉取 bom_quote_cache。
func dedupeQuoteCachePairs(lines []data.BomSessionLine, plats []string) []biz.MpnPlatformPair {
	seen := make(map[string]struct{})
	var out []biz.MpnPlatformPair
	for _, line := range lines {
		keys := []string{biz.NormalizeMPNForBOMSearch(line.Mpn)}
		if sub := biz.NormalizeMPNForBOMSearch(derefStrPtr(line.SubstituteMpn)); sub != "" && sub != keys[0] {
			keys = append(keys, sub)
		}
		for _, mk := range keys {
			for _, pid := range plats {
				pid = biz.NormalizePlatformID(pid)
				k := quoteCachePairKey(mk, pid)
				if _, ok := seen[k]; ok {
					continue
				}
				seen[k] = struct{}{}
				out = append(out, biz.MpnPlatformPair{MpnNorm: mk, PlatformID: pid})
			}
		}
	}
	return out
}

// sessionAliasCache 单次配单/搜价请求内缓存 CanonicalID，避免报价行维度的重复查表。
type sessionAliasCache struct {
	u  biz.AliasLookup
	mu sync.Mutex
	m  map[string]aliasCacheEntry
}

type aliasCacheEntry struct {
	id string
	ok bool
}

func newSessionAliasCache(u biz.AliasLookup) biz.AliasLookup {
	if u == nil {
		return nil
	}
	return &sessionAliasCache{u: u, m: make(map[string]aliasCacheEntry)}
}

func (s *sessionAliasCache) CanonicalID(ctx context.Context, aliasNorm string) (canonicalID string, ok bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, hit := s.m[aliasNorm]; hit {
		return e.id, e.ok, nil
	}
	id, ok, err := s.u.CanonicalID(ctx, aliasNorm)
	if err != nil {
		return "", false, err
	}
	s.m[aliasNorm] = aliasCacheEntry{id: id, ok: ok}
	return id, ok, nil
}

// sessionFXCache 单次请求内缓存汇率查询结果（含 Frankfurter 回源后的最终 ok）。
type sessionFXCache struct {
	u  biz.FXRateLookup
	mu sync.Mutex
	m  map[string]fxCacheEntry
}

type fxCacheEntry struct {
	rate         float64
	tableVersion string
	source       string
	ok           bool
}

func newSessionFXCache(u biz.FXRateLookup) biz.FXRateLookup {
	if u == nil {
		return nil
	}
	return &sessionFXCache{u: u, m: make(map[string]fxCacheEntry)}
}

func fxRateCacheKey(from, to string, date time.Time) string {
	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	y, m, d := date.In(time.UTC).Date()
	return from + "\x00" + to + "\x00" + fmt.Sprintf("%04d-%02d-%02d", y, m, d)
}

func (s *sessionFXCache) Rate(ctx context.Context, from, to string, date time.Time) (rate float64, tableVersion, source string, ok bool, err error) {
	key := fxRateCacheKey(from, to, date)
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, hit := s.m[key]; hit {
		return e.rate, e.tableVersion, e.source, e.ok, nil
	}
	rate, tv, src, ok, err := s.u.Rate(ctx, from, to, date)
	if err != nil {
		return 0, "", "", false, err
	}
	s.m[key] = fxCacheEntry{rate: rate, tableVersion: tv, source: src, ok: ok}
	return rate, tv, src, ok, nil
}
