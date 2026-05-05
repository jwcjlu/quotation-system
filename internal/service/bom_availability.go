package service

import (
	"context"
	"strings"
	"time"

	"caichip/internal/biz"
	"caichip/internal/data"

	"golang.org/x/sync/errgroup"
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

	matchTiming := bomMatchTimingEnabled()
	tPrefetch := time.Now()

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
	if matchTiming {
		s.log.Infof("bom_match_timing session=%s phase=availability_prefetch tasks=%d cache_entries=%d ms=%d",
			sessionID, len(tasks), len(cacheMap), time.Since(tPrefetch).Milliseconds())
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

	tFacts := time.Now()
	nLines := len(lines)
	// 与原先内层 for 一致：按 plats 顺序只保留非空规范化平台 ID。
	pidsInOrder := make([]string, 0, len(plats))
	for _, platformID := range plats {
		pid := biz.NormalizePlatformID(platformID)
		if pid == "" {
			continue
		}
		pidsInOrder = append(pidsInOrder, pid)
	}
	nP := len(pidsInOrder)

	mpnByLine := make([]string, nLines)
	lineNoByLine := make([]int, nLines)
	for i, line := range lines {
		mpnByLine[i] = biz.NormalizeMPNForBOMSearch(line.Mpn)
		lineNoByLine[i] = line.LineNo
	}

	slotFacts := make([][]biz.PlatformAvailabilityFact, nLines)
	for i := range lines {
		slotFacts[i] = make([]biz.PlatformAvailabilityFact, nP)
	}

	nJobs := nLines * nP
	cellWorkers := nJobs
	if cellWorkers > maxBomMatchLineWorkers*2 {
		cellWorkers = maxBomMatchLineWorkers * 2
	}
	if cellWorkers < 1 {
		cellWorkers = 1
	}
	eg, gctx := errgroup.WithContext(ctx)
	eg.SetLimit(cellWorkers)
	for lineIdx := range lines {
		lineIdx, line := lineIdx, lines[lineIdx]
		for platIdx, pid := range pidsInOrder {
			platIdx, pid := platIdx, pid
			lineCopy := line
			eg.Go(func() error {
				if err := gctx.Err(); err != nil {
					return err
				}
				mpnNorm := mpnByLine[lineIdx]
				key := quoteCachePairKey(mpnNorm, pid)
				fact, err := s.platformAvailabilityFact(
					gctx, sessionID, lineCopy, pid, taskState[key], cacheMap[key], bizDate, plats, aliasCache, fxCache,
				)
				if err != nil {
					return err
				}
				slotFacts[lineIdx][platIdx] = fact
				return nil
			})
		}
	}
	if err := eg.Wait(); err != nil {
		return nil, biz.LineAvailabilitySummary{}, err
	}
	out := make([]biz.LineAvailability, 0, nLines)
	for i := 0; i < nLines; i++ {
		out = append(out, biz.ClassifyLineAvailability(biz.LineAvailabilityInput{
			LineNo:    lineNoByLine[i],
			MpnNorm:   mpnByLine[i],
			Platforms: slotFacts[i],
		}))
	}
	if matchTiming {
		s.log.Infof("bom_match_timing session=%s phase=availability_line_platform_facts lines=%d platforms=%d ms=%d",
			sessionID, len(lines), len(plats), time.Since(tFacts).Milliseconds())
	}
	return out, biz.SummarizeLineAvailability(out), nil
}

func (s *BomService) platformAvailabilityFact(ctx context.Context, sessionID string, line data.BomSessionLine, pid, state string, snap *biz.QuoteCacheSnapshot, bizDate time.Time, sessionPlats []string, aliasCache biz.AliasLookup, fxCache biz.FXRateLookup) (biz.PlatformAvailabilityFact, error) {
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
		ok, err := s.platformHasUsableQuote(ctx, sessionID, line, pid, snap, bizDate, sessionPlats, aliasCache, fxCache)
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

func (s *BomService) platformHasUsableQuote(ctx context.Context, sessionID string, line data.BomSessionLine, pid string, snap *biz.QuoteCacheSnapshot, bizDate time.Time, sessionPlats []string, aliasCache biz.AliasLookup, fxCache biz.FXRateLookup) (bool, error) {
	if fxCache == nil || aliasCache == nil {
		return false, nil
	}
	baseRows, ok := parseQuoteRowsForMatch(snap.QuotesJSON)
	if !ok {
		return false, nil
	}
	var mfrHint *biz.BomManufacturerResolveHint
	if line.ManufacturerCanonicalID != nil && strings.TrimSpace(*line.ManufacturerCanonicalID) != "" {
		mfrHint = &biz.BomManufacturerResolveHint{CanonID: strings.TrimSpace(*line.ManufacturerCanonicalID), Hit: true}
	} else if mfr := strings.TrimSpace(derefStrPtr(line.Mfr)); mfr != "" {
		id, hit, err := biz.ResolveManufacturerCanonical(ctx, mfr, aliasCache)
		if err != nil {
			return false, err
		}
		mfrHint = &biz.BomManufacturerResolveHint{CanonID: id, Hit: hit}
	}
	runPick := func(model string, mergeMpnKey string) (biz.LineMatchPick, error) {
		quoteRows := append([]biz.AgentQuoteRow(nil), baseRows...)
		if s.search != nil && s.search.DBOk() && sessionID != "" && line.ID > 0 {
			dbRows, err := s.search.ListBomQuoteItemsForSessionLineRead(ctx, sessionID, line.ID, bizDate, mergeMpnKey, sessionPlats)
			if err != nil {
				return biz.LineMatchPick{}, err
			}
			quoteRows = mergeQuoteRowsWithSessionLineReads(quoteRows, dbRows, pid)
		}
		return biz.PickBestQuoteForLine(ctx, biz.LineMatchInput{
			BomMpn:           model,
			BomPackage:       derefStrPtr(line.Package),
			BomMfr:           derefStrPtr(line.Mfr),
			BomQty:           bomLineQtyInt(line.Qty),
			PlatformID:       pid,
			QuoteRows:        quoteRows,
			BizDate:          bizDate,
			RequestDay:       time.Now(),
			BaseCCY:          s.bomMatchBaseCCY(),
			RoundingMode:     s.bomMatchRoundingMode(),
			ParseTierStrings: s.bomMatchParseTiers(),
			BomMfrHint:       mfrHint,
		}, fxCache, aliasCache)
	}
	mergePrimary := biz.NormalizeMPNForBOMSearch(line.Mpn)
	pick, err := runPick(line.Mpn, mergePrimary)
	if err != nil {
		return false, err
	}
	if !pick.Ok {
		sub := strings.TrimSpace(derefStrPtr(line.SubstituteMpn))
		if sub != "" && !strings.EqualFold(sub, line.Mpn) {
			pick, err = runPick(sub, biz.NormalizeMPNForBOMSearch(sub))
			if err != nil {
				return false, err
			}
		}
	}
	return pick.Ok, nil
}
