package service

import (
	"context"
	"errors"
	"strings"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/data"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"gorm.io/gorm"
)

// SessionLineMfrCandidatesReply 阶段一：需求行厂牌待清洗列表（JSON HTTP）。
type SessionLineMfrCandidatesReply struct {
	Items []SessionLineMfrCandidate `json:"items"`
}

type SessionLineMfrCandidate struct {
	LineNo                 int    `json:"line_no"`
	Mfr                    string `json:"mfr"`
	RecommendedCanonicalID string `json:"recommended_canonical_id"`
}

// QuoteItemMfrReviewsReply 阶段二列表（含闸门）；见 SRS REQ-API-003。
type QuoteItemMfrReviewsReply struct {
	GateOpen                bool                     `json:"gate_open"`
	Items                   []QuoteItemMfrReviewItem `json:"items"`
	AllPendingQuoteMfrCount int32                    `json:"all_pending_quote_mfr_count"`
}

type QuoteItemMfrReviewItem struct {
	QuoteItemID                 uint64 `json:"quote_item_id"`
	LineNo                      int    `json:"line_no"`
	LineManufacturerCanonicalID string `json:"line_manufacturer_canonical_id"`
	Manufacturer                string `json:"manufacturer"`
}

// SubmitQuoteItemMfrReviewBody 阶段二提交。
type SubmitQuoteItemMfrReviewBody struct {
	Decision string `json:"decision"` // accept | reject
	Reason   string `json:"reason,omitempty"`
}

// listSessionLineMfrCandidatesInternal 阶段一候选（仅需求行，不扫报价 JSON）。
func (s *BomService) listSessionLineMfrCandidatesInternal(ctx context.Context, sessionID string) (*SessionLineMfrCandidatesReply, error) {
	if !s.dbOK() || s.alias == nil || !s.alias.DBOk() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, kerrors.BadRequest("BAD_INPUT", "session_id required")
	}
	_, lines, _, err := s.loadSessionLinesAndPlatforms(ctx, sessionID, nil)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, kerrors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		return nil, err
	}
	snaps := make([]biz.LinePhase1CleaningSnap, 0, len(lines))
	for _, line := range lines {
		snaps = append(snaps, biz.LinePhase1CleaningSnap{
			LineNo:                  line.LineNo,
			Mfr:                     line.Mfr,
			ManufacturerCanonicalID: line.ManufacturerCanonicalID,
		})
	}
	needs := biz.SessionLinesNeedingPhase1MfrCleaning(snaps)
	out := make([]SessionLineMfrCandidate, 0, len(needs))
	for _, n := range needs {
		rec, _, err := biz.ResolveManufacturerCanonical(ctx, n.Mfr, s.alias)
		if err != nil {
			return nil, err
		}
		out = append(out, SessionLineMfrCandidate{
			LineNo:                 n.LineNo,
			Mfr:                    n.Mfr,
			RecommendedCanonicalID: rec,
		})
	}
	return &SessionLineMfrCandidatesReply{Items: out}, nil
}

func filterQuoteItemMfrReviewsByPriorityLineSets(
	items []QuoteItemMfrReviewItem,
	byLine map[int]map[uint64]struct{},
) []QuoteItemMfrReviewItem {
	if len(byLine) == 0 {
		return items
	}
	out := make([]QuoteItemMfrReviewItem, 0, len(items))
	for _, it := range items {
		set, ok := byLine[it.LineNo]
		if !ok {
			out = append(out, it)
			continue
		}
		if _, hit := set[it.QuoteItemID]; hit {
			out = append(out, it)
		}
	}
	return out
}

// listQuoteItemMfrReviewsInternal 阶段二待评审列表 + 闸门。
// 待评审列表仅基于 quote-item-mfr-reviews 自身读链路计算，不依赖 readiness 的 TopK/TopN 子集。
func (s *BomService) listQuoteItemMfrReviewsInternal(ctx context.Context, sessionID string, includeAll bool) (*QuoteItemMfrReviewsReply, error) {
	if !s.dbOK() || s.mfrCleaning == nil || !s.mfrCleaning.DBOk() || s.alias == nil || !s.alias.DBOk() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, kerrors.BadRequest("BAD_INPUT", "session_id required")
	}
	view, lines, plats, err := s.loadSessionLinesAndPlatforms(ctx, sessionID, nil)
	if err != nil {
		return nil, err
	}
	gateSnapshots := make([]biz.LineMfrGateSnapshot, 0, len(lines))
	for _, line := range lines {
		gateSnapshots = append(gateSnapshots, biz.LineMfrGateSnapshot{
			LineNo:                  line.LineNo,
			Mfr:                     line.Mfr,
			ManufacturerCanonicalID: line.ManufacturerCanonicalID,
		})
	}
	gateOpen := biz.SessionLineMfrGateOpen(gateSnapshots)
	reply := &QuoteItemMfrReviewsReply{GateOpen: gateOpen, Items: nil}
	if !gateOpen {
		reply.Items = []QuoteItemMfrReviewItem{}
		return reply, nil
	}
	pendings, err := s.listMfrReviewPendingQuoteItemsMerged(ctx, sessionID, view, lines, plats)
	if err != nil {
		return nil, err
	}
	items := make([]QuoteItemMfrReviewItem, 0)
	for _, pending := range pendings {
		items = append(items, QuoteItemMfrReviewItem{
			QuoteItemID:                 pending.ID,
			LineNo:                      int(*pending.LineID),
			LineManufacturerCanonicalID: strings.TrimSpace(*pending.ManufacturerCanonicalID),
			Manufacturer:                *pending.ManufacturerCanonicalID,
		})
	}
	allPending := int32(len(items))
	reply.Items = items
	reply.AllPendingQuoteMfrCount = allPending
	return reply, nil
}

// submitQuoteItemMfrReviewInternal 阶段二通过 / 不通过（支持改判）。
func (s *BomService) submitQuoteItemMfrReviewInternal(ctx context.Context, sessionID string, quoteItemID uint64, body SubmitQuoteItemMfrReviewBody) error {
	if !s.dbOK() || s.mfrCleaning == nil || !s.mfrCleaning.DBOk() {
		return kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	dec := strings.ToLower(strings.TrimSpace(body.Decision))
	if sessionID == "" || quoteItemID == 0 {
		return kerrors.BadRequest("BAD_INPUT", "session_id and quote_item_id required")
	}
	if dec != "accept" && dec != "reject" {
		return kerrors.BadRequest("BAD_INPUT", "decision must be accept or reject")
	}
	it, err := s.mfrCleaning.LoadMfrReviewQuoteItem(ctx, sessionID, quoteItemID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if it == nil || errors.Is(err, gorm.ErrRecordNotFound) {
		it2, err2 := s.loadMfrReviewQuoteItemViaCacheRead(ctx, sessionID, quoteItemID)
		if err2 != nil && !errors.Is(err2, gorm.ErrRecordNotFound) {
			return err2
		}
		if it2 == nil {
			return kerrors.NotFound("QUOTE_ITEM_NOT_FOUND", "quote item not found")
		}
		it = it2
	}
	lines, err := s.dataListLines(ctx, sessionID)
	if err != nil {
		return err
	}
	if it.LineID == nil {
		return kerrors.BadRequest("BAD_INPUT", "quote item missing line_id")
	}
	var parent *data.BomSessionLine
	for i := range lines {
		if lines[i].ID == *it.LineID {
			parent = &lines[i]
			break
		}
	}
	if parent == nil {
		return kerrors.BadRequest("BAD_INPUT", "parent line not found")
	}
	if err := biz.RequireParentManufacturerCanonicalForQuoteMfrReview(parent.ManufacturerCanonicalID); err != nil {
		return kerrors.BadRequest("GATE", err.Error())
	}
	lineCanon := strings.TrimSpace(*parent.ManufacturerCanonicalID)
	if dec == "accept" {
		canon := lineCanon
		reason := (*string)(nil)
		return s.mfrCleaning.UpdateQuoteItemManufacturerReview(ctx, quoteItemID, biz.MfrReviewAccepted, &canon, reason)
	}
	var reasonPtr *string
	if strings.TrimSpace(body.Reason) != "" {
		r := strings.TrimSpace(body.Reason)
		reasonPtr = &r
	}
	return s.mfrCleaning.UpdateQuoteItemManufacturerReview(ctx, quoteItemID, biz.MfrReviewRejected, nil, reasonPtr)
}

// ListSessionLineMfrCandidates 实现 api.bom.v1.BomService（proto HTTP）。
func (s *BomService) ListSessionLineMfrCandidates(ctx context.Context, req *v1.ListSessionLineMfrCandidatesRequest) (*v1.ListSessionLineMfrCandidatesReply, error) {
	core, err := s.listSessionLineMfrCandidatesInternal(ctx, strings.TrimSpace(req.GetSessionId()))
	if err != nil {
		return nil, err
	}
	items := make([]*v1.SessionLineMfrCandidateRow, 0, len(core.Items))
	for _, it := range core.Items {
		items = append(items, &v1.SessionLineMfrCandidateRow{
			LineNo:                 int32(it.LineNo),
			Mfr:                    it.Mfr,
			RecommendedCanonicalId: it.RecommendedCanonicalID,
		})
	}
	return &v1.ListSessionLineMfrCandidatesReply{Items: items}, nil
}

// ListQuoteItemMfrReviews 实现 api.bom.v1.BomService（proto HTTP）。
func (s *BomService) ListQuoteItemMfrReviews(ctx context.Context, req *v1.ListQuoteItemMfrReviewsRequest) (*v1.ListQuoteItemMfrReviewsReply, error) {
	core, err := s.listQuoteItemMfrReviewsInternal(ctx, strings.TrimSpace(req.GetSessionId()), req.GetIncludeAllPendingQuoteMfr())
	if err != nil {
		return nil, err
	}
	out := &v1.ListQuoteItemMfrReviewsReply{
		GateOpen:                core.GateOpen,
		Items:                   make([]*v1.QuoteItemMfrReviewRow, 0, len(core.Items)),
		AllPendingQuoteMfrCount: core.AllPendingQuoteMfrCount,
	}
	for _, it := range core.Items {
		out.Items = append(out.Items, &v1.QuoteItemMfrReviewRow{
			QuoteItemId:                 it.QuoteItemID,
			LineNo:                      int32(it.LineNo),
			LineManufacturerCanonicalId: it.LineManufacturerCanonicalID,
			Manufacturer:                it.Manufacturer,
			PlatformId:                  "",
		})
	}
	return out, nil
}

// SubmitQuoteItemMfrReview 实现 api.bom.v1.BomService（proto HTTP）。
func (s *BomService) SubmitQuoteItemMfrReview(ctx context.Context, req *v1.SubmitQuoteItemMfrReviewRequest) (*v1.SubmitQuoteItemMfrReviewReply, error) {
	body := SubmitQuoteItemMfrReviewBody{
		Decision: strings.TrimSpace(req.GetDecision()),
		Reason:   req.GetReason(),
	}
	if err := s.submitQuoteItemMfrReviewInternal(ctx, strings.TrimSpace(req.GetSessionId()), req.GetQuoteItemId(), body); err != nil {
		return nil, err
	}
	return &v1.SubmitQuoteItemMfrReviewReply{}, nil
}
