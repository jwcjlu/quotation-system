package service

import (
	"context"
	"strings"
	"time"

	"caichip/internal/biz"
	"caichip/internal/data"
)

func (s *BomService) computeLineAvailability(ctx context.Context, view *biz.BOMSessionView, lines []data.BomSessionLine, plats []string) ([]biz.LineAvailability, biz.LineAvailabilitySummary, error) {
	var sessionID string
	var bizDate time.Time
	if view != nil {
		sessionID = view.SessionID
		bizDate = view.BizDate
	}
	aliasCache := newSessionAliasCache(s.alias)
	fxCache := newSessionFXCache(s.fx)

	var tasks []biz.TaskReadinessSnapshot
	cacheMap := map[string]*biz.QuoteCacheSnapshot{}
	if s.search != nil {
		var err error
		tasks, err = s.search.ListTasksForSession(ctx, sessionID)
		if err != nil {
			return nil, biz.LineAvailabilitySummary{}, err
		}
		cacheMap, err = s.search.LoadQuoteCachesForKeys(ctx, bizDate, dedupeQuoteCachePairs(lines, plats))
		if err != nil {
			return nil, biz.LineAvailabilitySummary{}, err
		}
	}

	taskState := make(map[string]string, len(tasks))
	for _, task := range tasks {
		mpnNorm := biz.NormalizeMPNForBOMSearch(task.MpnNorm)
		pid := biz.NormalizePlatformID(task.PlatformID)
		if mpnNorm == "" || pid == "" {
			continue
		}
		taskState[quoteCachePairKey(mpnNorm, pid)] = strings.ToLower(strings.TrimSpace(task.State))
	}

	out := make([]biz.LineAvailability, 0, len(lines))
	for _, line := range lines {
		mpnNorm := biz.NormalizeMPNForBOMSearch(line.Mpn)
		facts := make([]biz.PlatformAvailabilityFact, 0, len(plats))
		for _, platformID := range plats {
			pid := biz.NormalizePlatformID(platformID)
			if pid == "" {
				continue
			}
			key := quoteCachePairKey(mpnNorm, pid)
			fact, err := s.platformAvailabilityFact(ctx, line, pid, taskState[key], cacheMap[key], bizDate, aliasCache, fxCache)
			if err != nil {
				return nil, biz.LineAvailabilitySummary{}, err
			}
			facts = append(facts, fact)
		}
		out = append(out, biz.ClassifyLineAvailability(biz.LineAvailabilityInput{
			LineNo:    line.LineNo,
			MpnNorm:   mpnNorm,
			Platforms: facts,
		}))
	}
	return out, biz.SummarizeLineAvailability(out), nil
}

func (s *BomService) platformAvailabilityFact(ctx context.Context, line data.BomSessionLine, pid, state string, snap *biz.QuoteCacheSnapshot, bizDate time.Time, aliasCache biz.AliasLookup, fxCache biz.FXRateLookup) (biz.PlatformAvailabilityFact, error) {
	state = strings.ToLower(strings.TrimSpace(state))
	fact := biz.PlatformAvailabilityFact{
		PlatformID: pid,
		TaskState:  state,
	}

	if snap != nil {
		switch strings.ToLower(strings.TrimSpace(snap.Outcome)) {
		case "no_mpn_match", "no_result":
			fact.NoData = true
			fact.ReasonCode = "NO_MPN"
		}
	}

	switch state {
	case "no_result":
		fact.NoData = true
		fact.ReasonCode = "NO_MPN"
	case "failed_terminal":
		fact.CollectionUnavailable = true
		fact.ReasonCode = "FETCH_FAILED"
	case "cancelled", "skipped":
		fact.CollectionUnavailable = true
		fact.ReasonCode = "PLATFORM_SKIPPED"
	}
	if fact.NoData || fact.CollectionUnavailable {
		return fact, nil
	}

	if quoteCacheUsable(snap) {
		fact.HasRawQuote = true
		ok, err := s.platformHasUsableQuote(ctx, line, pid, snap, bizDate, aliasCache, fxCache)
		if err != nil {
			return fact, err
		}
		fact.HasUsableQuote = ok
		if ok {
			fact.ReasonCode = "READY"
		} else {
			fact.ReasonCode = "FILTERED_BY_MFR"
		}
	}
	return fact, nil
}

func (s *BomService) platformHasUsableQuote(ctx context.Context, line data.BomSessionLine, pid string, snap *biz.QuoteCacheSnapshot, bizDate time.Time, aliasCache biz.AliasLookup, fxCache biz.FXRateLookup) (bool, error) {
	if fxCache == nil || aliasCache == nil {
		return false, nil
	}
	rows, ok := parseQuoteRowsForMatch(snap.QuotesJSON)
	if !ok {
		return false, nil
	}
	var mfrHint *biz.BomManufacturerResolveHint
	if mfr := strings.TrimSpace(derefStrPtr(line.Mfr)); mfr != "" {
		id, hit, err := biz.ResolveManufacturerCanonical(ctx, mfr, aliasCache)
		if err != nil {
			return false, err
		}
		mfrHint = &biz.BomManufacturerResolveHint{CanonID: id, Hit: hit}
	}
	runPick := func(model string) (biz.LineMatchPick, error) {
		return biz.PickBestQuoteForLine(ctx, biz.LineMatchInput{
			BomMpn:           model,
			BomPackage:       derefStrPtr(line.Package),
			BomMfr:           derefStrPtr(line.Mfr),
			BomQty:           bomLineQtyInt(line.Qty),
			PlatformID:       pid,
			QuoteRows:        rows,
			BizDate:          bizDate,
			RequestDay:       time.Now(),
			BaseCCY:          s.bomMatchBaseCCY(),
			RoundingMode:     s.bomMatchRoundingMode(),
			ParseTierStrings: s.bomMatchParseTiers(),
			BomMfrHint:       mfrHint,
		}, fxCache, aliasCache)
	}
	pick, err := runPick(line.Mpn)
	if err != nil {
		return false, err
	}
	if !pick.Ok {
		sub := strings.TrimSpace(derefStrPtr(line.SubstituteMpn))
		if sub != "" && !strings.EqualFold(sub, line.Mpn) {
			pick, err = runPick(sub)
			if err != nil {
				return false, err
			}
		}
	}
	return pick.Ok, nil
}
