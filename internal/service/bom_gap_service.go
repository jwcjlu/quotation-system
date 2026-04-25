package service

import (
	"context"
	"strconv"
	"strings"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func (s *BomService) syncLineGaps(ctx context.Context, view *biz.BOMSessionView) error {
	if s.gaps == nil || !s.gaps.DBOk() || view == nil {
		return nil
	}
	lines, err := s.dataListLines(ctx, view.SessionID)
	if err != nil {
		return err
	}
	tasks, err := s.search.ListTasksForSession(ctx, view.SessionID)
	if err != nil {
		return err
	}
	taskByKey := make(map[string]biz.TaskReadinessSnapshot, len(tasks))
	for _, t := range tasks {
		taskByKey[quoteCachePairKey(t.MpnNorm, t.PlatformID)] = t
	}
	pairs := dedupeQuoteCachePairs(lines, view.PlatformIDs)
	cacheMap, err := s.search.LoadQuoteCachesForKeys(ctx, view.BizDate, pairs)
	if err != nil {
		return err
	}
	var gaps []biz.BOMLineGap
	for _, line := range lines {
		mn := biz.NormalizeMPNForBOMSearch(line.Mpn)
		status, reasonCode, reason := s.lineGapAvailability(mn, view.PlatformIDs, taskByKey, cacheMap)
		gt := biz.AvailabilityStatusToGapType(status)
		if gt == "" {
			continue
		}
		gaps = append(gaps, biz.BOMLineGap{
			SessionID: view.SessionID, LineID: line.ID, LineNo: line.LineNo,
			Mpn: mn, GapType: gt, ReasonCode: reasonCode, ReasonDetail: reason, Status: biz.LineGapOpen,
		})
	}
	return s.gaps.UpsertOpenGaps(ctx, gaps)
}

func (s *BomService) lineGapAvailability(mpnNorm string, platforms []string, tasks map[string]biz.TaskReadinessSnapshot, cacheMap map[string]*biz.QuoteCacheSnapshot) (string, string, string) {
	if len(platforms) == 0 {
		return biz.LineAvailabilityCollectionUnavailable, "NO_PLATFORM", "no selected platforms"
	}
	anyTask := false
	allNoData := true
	for _, p := range platforms {
		pid := biz.NormalizePlatformID(p)
		key := quoteCachePairKey(mpnNorm, pid)
		if _, ok := tasks[key]; ok {
			anyTask = true
		}
		snap := cacheMap[key]
		if quoteCacheUsable(snap) {
			return "", "", ""
		}
		if snap == nil || strings.TrimSpace(snap.Outcome) == "" {
			allNoData = false
		}
	}
	if !anyTask {
		return biz.LineAvailabilityCollectionUnavailable, "TASK_MISSING", "selected platforms have no search tasks"
	}
	if allNoData {
		return biz.LineAvailabilityNoData, "NO_DATA", "all selected platforms returned no data"
	}
	return biz.LineAvailabilityNoMatchAfterFilter, "NO_MATCH_AFTER_FILTER", "quote rows did not pass BOM filters"
}

func (s *BomService) ListLineGaps(ctx context.Context, req *v1.ListLineGapsRequest) (*v1.ListLineGapsReply, error) {
	if s.gaps == nil || !s.gaps.DBOk() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "gap repo not configured")
	}
	view, err := s.session.GetSession(ctx, req.GetSessionId())
	if err != nil {
		return nil, err
	}
	if err := s.syncLineGaps(ctx, view); err != nil {
		s.log.Warnf("sync line gaps: %v", err)
	}
	gaps, err := s.gaps.ListLineGaps(ctx, req.GetSessionId(), req.GetStatuses())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.BOMLineGap, 0, len(gaps))
	for _, g := range gaps {
		out = append(out, &v1.BOMLineGap{
			GapId: strconv.FormatUint(g.ID, 10), SessionId: g.SessionID,
			LineId: strconv.FormatInt(g.LineID, 10), LineNo: int32(g.LineNo),
			Mpn: g.Mpn, GapType: g.GapType, ReasonCode: g.ReasonCode,
			ReasonDetail: g.ReasonDetail, ResolutionStatus: g.Status,
			SubstituteMpn: g.SubstituteMpn, SubstituteReason: g.SubstituteReason,
		})
	}
	return &v1.ListLineGapsReply{Gaps: out}, nil
}

func (s *BomService) ResolveLineGapManualQuote(ctx context.Context, req *v1.ResolveLineGapManualQuoteRequest) (*v1.ResolveLineGapManualQuoteReply, error) {
	gapID, err := strconv.ParseUint(req.GetGapId(), 10, 64)
	if err != nil {
		return nil, kerrors.BadRequest("BAD_GAP_ID", "invalid gap_id")
	}
	if strings.TrimSpace(req.GetModel()) == "" {
		return nil, kerrors.BadRequest("BAD_MANUAL_QUOTE", "model required")
	}
	if err := s.search.UpsertManualQuote(ctx, gapID, biz.AgentQuoteRow{
		Model: req.GetModel(), Manufacturer: req.GetManufacturer(), Package: req.GetPackage(),
		Stock: req.GetStock(), LeadTime: req.GetLeadTime(), PriceTiers: req.GetPriceTiers(),
		HKPrice: req.GetHkPrice(), MainlandPrice: req.GetMainlandPrice(),
	}); err != nil {
		return nil, err
	}
	if err := s.gaps.UpdateLineGapStatus(ctx, gapID, biz.LineGapOpen, biz.LineGapManualQuoteAdded, "", req.GetNote()); err != nil {
		return nil, err
	}
	return &v1.ResolveLineGapManualQuoteReply{Accepted: true}, nil
}

func (s *BomService) SelectLineGapSubstitute(ctx context.Context, req *v1.SelectLineGapSubstituteRequest) (*v1.SelectLineGapSubstituteReply, error) {
	gapID, err := strconv.ParseUint(req.GetGapId(), 10, 64)
	if err != nil {
		return nil, kerrors.BadRequest("BAD_GAP_ID", "invalid gap_id")
	}
	sub := strings.TrimSpace(req.GetSubstituteMpn())
	if sub == "" {
		return nil, kerrors.BadRequest("BAD_SUBSTITUTE", "substitute_mpn required")
	}
	gap, err := s.gaps.GetLineGap(ctx, gapID)
	if err != nil {
		return nil, kerrors.NotFound("GAP_NOT_FOUND", "gap not found")
	}
	if gap.Status != biz.LineGapOpen {
		return nil, kerrors.Conflict("GAP_NOT_OPEN", "gap is not open")
	}
	view, err := s.session.GetSession(ctx, gap.SessionID)
	if err != nil {
		return nil, err
	}
	mn := biz.NormalizeMPNForBOMSearch(sub)
	pairs := make([]biz.MpnPlatformPair, 0, len(view.PlatformIDs))
	for _, p := range view.PlatformIDs {
		pairs = append(pairs, biz.MpnPlatformPair{MpnNorm: mn, PlatformID: biz.NormalizePlatformID(p)})
	}
	if err := s.search.UpsertPendingTasks(ctx, gap.SessionID, view.BizDate, view.SelectionRevision, pairs); err != nil {
		return nil, err
	}
	if err := s.gaps.SelectLineGapSubstitute(ctx, gapID, "", sub, req.GetReason()); err != nil {
		return nil, err
	}
	s.tryMergeDispatchSession(ctx, gap.SessionID)
	return &v1.SelectLineGapSubstituteReply{Accepted: true}, nil
}
