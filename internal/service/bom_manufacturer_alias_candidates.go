package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"caichip/internal/biz"
	"caichip/internal/data"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"gorm.io/gorm"
)

type ManufacturerAliasCandidate struct {
	Kind                   string   `json:"kind"`
	Alias                  string   `json:"alias"`
	RecommendedCanonicalID string   `json:"recommended_canonical_id"`
	LineNos                []int    `json:"line_nos"`
	PlatformIDs            []string `json:"platform_ids"`
	DemandHint             string   `json:"demand_hint"`
}

type ListManufacturerAliasCandidatesReply struct {
	Items []ManufacturerAliasCandidate `json:"items"`
}

type manufacturerAliasQuoteRowLister interface {
	ListManufacturerAliasQuoteRows(ctx context.Context, bizDate time.Time, pairs []biz.MpnPlatformPair) (map[string][]biz.AgentQuoteRow, error)
}

type manufacturerAliasCandidateGroup struct {
	kind        string
	alias       string
	recommended string
	lineNos     map[int]struct{}
	platforms   map[string]struct{}
	demand      map[string]struct{}
}

func (s *BomService) ListManufacturerAliasCandidates(ctx context.Context, sessionID string) (*ListManufacturerAliasCandidatesReply, error) {
	if !s.dbOK() || s.alias == nil || !s.alias.DBOk() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	view, lines, plats, err := s.loadSessionLinesAndPlatforms(ctx, strings.TrimSpace(sessionID), nil)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, kerrors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		return nil, err
	}
	quoteRowsByKey, err := s.loadManufacturerAliasQuoteRows(ctx, view.BizDate, dedupeQuoteCachePairs(lines, plats))
	if err != nil {
		return nil, err
	}
	aliasCache := newSessionAliasCache(s.alias)
	groups := make(map[string]*manufacturerAliasCandidateGroup)
	for _, line := range lines {
		if err := collectLineManufacturerAliasCandidates(ctx, line, plats, quoteRowsByKey, aliasCache, groups); err != nil {
			return nil, err
		}
	}
	return &ListManufacturerAliasCandidatesReply{Items: manufacturerAliasCandidateGroupsToRows(groups)}, nil
}

func (s *BomService) loadManufacturerAliasQuoteRows(ctx context.Context, bizDate time.Time, pairs []biz.MpnPlatformPair) (map[string][]biz.AgentQuoteRow, error) {
	if lister, ok := s.search.(manufacturerAliasQuoteRowLister); ok {
		return lister.ListManufacturerAliasQuoteRows(ctx, bizDate, pairs)
	}
	cacheMap, err := s.search.LoadQuoteCachesForKeys(ctx, bizDate, pairs)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]biz.AgentQuoteRow, len(cacheMap))
	for key, snap := range cacheMap {
		if !quoteCacheUsable(snap) {
			continue
		}
		rows, ok := parseQuoteRowsForMatch(snap.QuotesJSON)
		if ok {
			out[key] = rows
		}
	}
	return out, nil
}

func collectLineManufacturerAliasCandidates(ctx context.Context, line data.BomSessionLine, plats []string, quoteRowsByKey map[string][]biz.AgentQuoteRow, alias biz.AliasLookup, groups map[string]*manufacturerAliasCandidateGroup) error {
	demandMfr := strings.TrimSpace(derefStrPtr(line.Mfr))
	if demandMfr == "" || alias == nil {
		return nil
	}
	var demandCanon string
	if line.ManufacturerCanonicalID != nil && strings.TrimSpace(*line.ManufacturerCanonicalID) != "" {
		demandCanon = strings.TrimSpace(*line.ManufacturerCanonicalID)
	} else {
		id, hit, err := biz.ResolveManufacturerCanonical(ctx, demandMfr, alias)
		if err != nil {
			return err
		}
		if hit {
			demandCanon = id
		}
		addManufacturerAliasCandidate(groups, "demand", demandMfr, demandCanon, line.LineNo, plats, "")
	}
	mpnNorm := biz.NormalizeMPNForBOMSearch(line.Mpn)
	if demandCanon == "" {
		return nil
	}
	for _, platformID := range plats {
		pid := biz.NormalizePlatformID(platformID)
		if pid == "" {
			continue
		}
		rows := quoteRowsByKey[quoteCachePairKey(mpnNorm, pid)]
		for _, row := range rows {
			if !quoteRowMatchesLineModelAndPackage(line, row) {
				continue
			}
			quoteMfr := strings.TrimSpace(row.Manufacturer)
			if quoteMfr == "" {
				continue
			}
			if row.ManufacturerCanonicalID != nil && strings.TrimSpace(*row.ManufacturerCanonicalID) != "" {
				continue
			}
			quoteCanon, quoteHit, err := biz.ResolveManufacturerCanonical(ctx, quoteMfr, alias)
			if err != nil {
				return err
			}
			if quoteHit && quoteCanon == demandCanon {
				continue
			}
			addManufacturerAliasCandidate(groups, "quote", quoteMfr, demandCanon, line.LineNo, []string{pid}, demandMfr)
		}
	}
	return nil
}

func quoteRowMatchesLineModelAndPackage(line data.BomSessionLine, row biz.AgentQuoteRow) bool {
	if strings.TrimSpace(row.Model) == "" {
		return false
	}
	if biz.NormalizeMPNForBOMSearch(line.Mpn) != biz.NormalizeMPNForBOMSearch(row.Model) {
		return false
	}
	if pkg := strings.TrimSpace(derefStrPtr(line.Package)); pkg != "" {
		return biz.NormalizeMfrString(row.Package) == biz.NormalizeMfrString(pkg)
	}
	return true
}

func addManufacturerAliasCandidate(groups map[string]*manufacturerAliasCandidateGroup, kind, alias, recommended string, lineNo int, platformIDs []string, demandMfr string) {
	kind = strings.TrimSpace(kind)
	alias = strings.TrimSpace(alias)
	recommended = strings.TrimSpace(recommended)
	if kind == "" || alias == "" {
		return
	}
	key := kind + "\x00" + alias + "\x00" + recommended
	group := groups[key]
	if group == nil {
		group = &manufacturerAliasCandidateGroup{
			kind:        kind,
			alias:       alias,
			recommended: recommended,
			lineNos:     make(map[int]struct{}),
			platforms:   make(map[string]struct{}),
			demand:      make(map[string]struct{}),
		}
		groups[key] = group
	}
	if lineNo > 0 {
		group.lineNos[lineNo] = struct{}{}
	}
	for _, platformID := range platformIDs {
		pid := biz.NormalizePlatformID(platformID)
		if pid != "" {
			group.platforms[pid] = struct{}{}
		}
	}
	if demandMfr = strings.TrimSpace(demandMfr); demandMfr != "" {
		group.demand[demandMfr] = struct{}{}
	}
}

func manufacturerAliasCandidateGroupsToRows(groups map[string]*manufacturerAliasCandidateGroup) []ManufacturerAliasCandidate {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]ManufacturerAliasCandidate, 0, len(keys))
	for _, key := range keys {
		group := groups[key]
		lineNos := make([]int, 0, len(group.lineNos))
		for lineNo := range group.lineNos {
			lineNos = append(lineNos, lineNo)
		}
		sort.Ints(lineNos)
		platforms := make([]string, 0, len(group.platforms))
		for platformID := range group.platforms {
			platforms = append(platforms, platformID)
		}
		sort.Strings(platforms)
		demand := make([]string, 0, len(group.demand))
		for item := range group.demand {
			demand = append(demand, item)
		}
		sort.Strings(demand)
		out = append(out, ManufacturerAliasCandidate{
			Kind:                   group.kind,
			Alias:                  group.alias,
			RecommendedCanonicalID: group.recommended,
			LineNos:                lineNos,
			PlatformIDs:            platforms,
			DemandHint:             strings.Join(demand, ", "),
		})
	}
	return out
}
