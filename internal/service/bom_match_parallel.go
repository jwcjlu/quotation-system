package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/data"

	"github.com/panjf2000/ants/v2"
)

// maxBomMatchLineWorkers 配单时每会话并发处理 BOM 行的上限，避免过大并发压垮 DB / CPU。
const maxBomMatchLineWorkers = 32

// matchOneLine 处理单行×多平台选最优报价（线程安全前提：aliasCache / fxCache 带锁，cacheMap 只读）。
func (s *BomService) matchOneLine(
	ctx context.Context,
	sid string,
	line data.BomSessionLine,
	plats []string,
	cacheMap map[string]*biz.QuoteCacheSnapshot,
	aliasCache biz.AliasLookup,
	fxCache biz.FXRateLookup,
	view *biz.BOMSessionView,
	reqDay time.Time,
	baseCCY, roundMode string,
	parseTiers bool,
) (*v1.MatchItem, float64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	qtyI := bomLineQtyInt(line.Qty)
	type pc struct {
		pid  string
		pick biz.LineMatchPick
	}
	var cand []pc
	mergeKey := biz.NormalizeMPNForBOMSearch(line.Mpn)
	mfrSeenLine := make(map[string]struct{})
	var lineMfrMismatch []string
	addLineMfr := func(xs []string) {
		for _, x := range xs {
			if _, ok := mfrSeenLine[x]; ok {
				continue
			}
			mfrSeenLine[x] = struct{}{}
			lineMfrMismatch = append(lineMfrMismatch, x)
		}
	}
	var mfrHint *biz.BomManufacturerResolveHint
	if line.ManufacturerCanonicalID != nil && strings.TrimSpace(*line.ManufacturerCanonicalID) != "" {
		mfrHint = &biz.BomManufacturerResolveHint{CanonID: strings.TrimSpace(*line.ManufacturerCanonicalID), Hit: true}
	} else if mf := strings.TrimSpace(derefStrPtr(line.Mfr)); mf != "" {
		id, hit, rerr := biz.ResolveManufacturerCanonical(ctx, mf, aliasCache)
		if rerr != nil {
			return nil, 0, rerr
		}
		mfrHint = &biz.BomManufacturerResolveHint{CanonID: id, Hit: hit}
	}
	for _, pid := range plats {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		pid = biz.NormalizePlatformID(pid)
		snap := cacheMap[quoteCachePairKey(mergeKey, pid)]
		hit := snap != nil
		if !hit || !quoteCacheUsable(snap) {
			if r := quoteCacheUnusableReason(hit, snap); r != "" {
				oc := ""
				if snap != nil {
					oc = strings.TrimSpace(snap.Outcome)
				}
				s.log.Debugf(
					"bom match skip: session=%s line_no=%d mpn=%q merge_mpn=%q platform=%s reason=%s outcome=%q",
					sid, line.LineNo, line.Mpn, mergeKey, pid, r, oc,
				)
			}
			continue
		}
		rows, rowsOK := parseQuoteRowsForMatch(snap.QuotesJSON)
		if !rowsOK {
			s.log.Debugf(
				"bom match skip: session=%s line_no=%d mpn=%q merge_mpn=%q platform=%s reason=quotes_json_invalid_for_match",
				sid, line.LineNo, line.Mpn, mergeKey, pid,
			)
			continue
		}
		in := biz.LineMatchInput{
			BomMpn:           line.Mpn,
			BomPackage:       derefStrPtr(line.Package),
			BomMfr:           derefStrPtr(line.Mfr),
			BomQty:           qtyI,
			PlatformID:       pid,
			QuoteRows:        rows,
			BizDate:          view.BizDate,
			RequestDay:       reqDay,
			BaseCCY:          baseCCY,
			RoundingMode:     roundMode,
			ParseTierStrings: parseTiers,
			BomMfrHint:       mfrHint,
		}
		pick, err := biz.PickBestQuoteForLine(ctx, in, fxCache, aliasCache)
		if err != nil {
			return nil, 0, err
		}
		addLineMfr(pick.MfrMismatchQuoteManufacturers)
		if pick.Ok {
			cand = append(cand, pc{pid, pick})
		} else {
			s.log.Debugf(
				"bom match skip: session=%s line_no=%d mpn=%q merge_mpn=%q platform=%s reason=pick_not_ok pick_reason=%q bom_mfr=%q bom_pkg=%q",
				sid, line.LineNo, line.Mpn, mergeKey, pid, pick.Reason, derefStrPtr(line.Mfr), derefStrPtr(line.Package),
			)
		}
	}
	if len(cand) == 0 {
		s.log.Debugf(
			"bom match line no_match: session=%s line_no=%d mpn=%q merge_mpn=%q qty=%d bom_mfr=%q bom_pkg=%q (no platform produced a candidate after skips above)",
			sid, line.LineNo, line.Mpn, mergeKey, qtyI, derefStrPtr(line.Mfr), derefStrPtr(line.Package),
		)
		mi := noMatchItem(line, qtyI, lineMfrMismatch)
		return mi, 0, nil
	}
	bestIdx := 0
	bestKey := matchSortKeyFromPick(cand[0].pick, cand[0].pid, roundMode)
	for i := 1; i < len(cand); i++ {
		k := matchSortKeyFromPick(cand[i].pick, cand[i].pid, roundMode)
		if biz.LessMatchCandidate(k, bestKey) {
			bestKey = k
			bestIdx = i
		}
	}
	ch := cand[bestIdx]
	mi := matchItemFromPick(line, qtyI, ch.pick, ch.pid, lineMfrMismatch)
	return mi, mi.GetSubtotal(), nil
}

func (s *BomService) computeMatchItems(ctx context.Context, view *biz.BOMSessionView, lines []data.BomSessionLine, plats []string) ([]*v1.MatchItem, float64, error) {
	baseCCY := s.bomMatchBaseCCY()
	roundMode := s.bomMatchRoundingMode()
	parseTiers := s.bomMatchParseTiers()
	reqDay := time.Now()
	sid := strings.TrimSpace(view.SessionID)

	n := len(lines)
	if n == 0 {
		return nil, 0, nil
	}

	pairList := dedupeQuoteCachePairs(lines, plats)
	cacheMap, err := s.search.LoadQuoteCachesForKeys(ctx, view.BizDate, pairList)
	if err != nil {
		return nil, 0, err
	}
	aliasCache := newSessionAliasCache(s.alias)
	fxCache := newSessionFXCache(s.fx)

	// 行数较少时直接串行，避免协程池开销。
	if n == 1 {
		item, st, err := s.matchOneLine(ctx, sid, lines[0], plats, cacheMap, aliasCache, fxCache, view, reqDay, baseCCY, roundMode, parseTiers)
		if err != nil {
			return nil, 0, err
		}
		out := []*v1.MatchItem{item}
		s.attachCustomsToMatchItems(ctx, lines, out)
		return out, st, nil
	}

	workers := n
	if workers > maxBomMatchLineWorkers {
		workers = maxBomMatchLineWorkers
	}
	pool, err := ants.NewPool(workers)
	if err != nil {
		return nil, 0, err
	}
	defer pool.Release()

	items := make([]*v1.MatchItem, n)
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup

	setErr := func(e error) {
		if e == nil {
			return
		}
		mu.Lock()
		if firstErr == nil {
			firstErr = e
		}
		mu.Unlock()
	}

	for i, line := range lines {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		wg.Add(1)
		idx, line := i, line
		submitErr := pool.Submit(func() {
			defer wg.Done()
			if ctx.Err() != nil {
				return
			}
			mu.Lock()
			fe := firstErr
			mu.Unlock()
			if fe != nil {
				return
			}
			item, _, err := s.matchOneLine(ctx, sid, line, plats, cacheMap, aliasCache, fxCache, view, reqDay, baseCCY, roundMode, parseTiers)
			if err != nil {
				setErr(err)
				return
			}
			items[idx] = item
		})
		if submitErr != nil {
			wg.Done()
			return nil, 0, submitErr
		}
	}
	wg.Wait()
	if firstErr != nil {
		return nil, 0, firstErr
	}
	var total float64
	out := make([]*v1.MatchItem, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, items[i])
		total += items[i].GetSubtotal()
	}
	s.attachCustomsToMatchItems(ctx, lines, out)
	return out, total, nil
}

func parseQuoteRowsForMatch(raw []byte) ([]biz.AgentQuoteRow, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var rows []biz.AgentQuoteRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, false
	}
	if len(rows) == 0 {
		return nil, false
	}
	return rows, true
}
