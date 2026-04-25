package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/conf"
	"caichip/internal/data"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// BomService 实现 BOM HTTP API（api/bom/v1.BomServiceHTTPServer）。
type BomService struct {
	session    biz.BOMSessionRepo
	search     biz.BOMSearchTaskRepo
	merge      biz.MergeDispatchExecutor
	openai     *data.OpenAIChat
	fx         *data.BomFxRateRepo
	alias      biz.BomManufacturerAliasRepo
	hsMapping  biz.HsModelMappingRepo
	hsItem     biz.HsItemReadRepo
	hsTaxDaily biz.HsTaxRateDailyRepo
	hsTaxAPI   biz.TaxRateAPIFetcher
	bomMatch   *conf.BomMatch
	log        *log.Helper
}

// NewBomService ...
func NewBomService(
	session biz.BOMSessionRepo,
	search biz.BOMSearchTaskRepo,
	merge biz.MergeDispatchExecutor,
	openai *data.OpenAIChat,
	fx *data.BomFxRateRepo,
	alias biz.BomManufacturerAliasRepo,
	hsMapping biz.HsModelMappingRepo,
	hsItem biz.HsItemReadRepo,
	hsTaxDaily biz.HsTaxRateDailyRepo,
	hsTaxAPI biz.TaxRateAPIFetcher,
	bc *conf.Bootstrap,
	logger log.Logger,
) *BomService {
	var bm *conf.BomMatch
	if bc != nil {
		bm = bc.BomMatch
	}
	return &BomService{
		session:    session,
		search:     search,
		merge:      merge,
		openai:     openai,
		fx:         fx,
		alias:      alias,
		hsMapping:  hsMapping,
		hsItem:     hsItem,
		hsTaxDaily: hsTaxDaily,
		hsTaxAPI:   hsTaxAPI,
		bomMatch:   bm,
		log:        log.NewHelper(logger),
	}
}

func (s *BomService) tryMergeDispatchSession(ctx context.Context, sessionID string) {
	if s.merge == nil || !s.merge.DBOk() {
		return
	}
	_ = s.merge.TryDispatchPendingKeysForSession(ctx, sessionID)
}

func (s *BomService) dbOK() bool {
	return s.session != nil && s.session.DBOk() && s.search != nil && s.search.DBOk()
}

// matchDepsOK：SearchQuotes / AutoMatch / GetMatchResult 依赖报价缓存、汇率与厂牌别名表；有 DB 时 Wire 注入的 fx/alias 须非 nil 且 DBOk。
func (s *BomService) matchDepsOK() bool {
	if !s.dbOK() {
		return false
	}
	return s.fx != nil && s.fx.DBOk() && s.alias != nil && s.alias.DBOk()
}

func notImplemented(msg string) error {
	return kerrors.ServiceUnavailable("BOM_LEGACY", msg)
}

func (s *BomService) SearchQuotes(ctx context.Context, req *v1.SearchQuotesRequest) (*v1.SearchQuotesReply, error) {
	if !s.matchDepsOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sid, err := parseBomSessionID(req.GetBomId())
	if err != nil {
		return nil, err
	}
	view, lines, plats, err := s.loadSessionLinesAndPlatforms(ctx, sid, req.GetPlatforms())
	if err != nil {
		return nil, err
	}
	if err := s.matchReadinessError(ctx, sid, view, lines); err != nil {
		return nil, err
	}
	pairList := dedupeQuoteCachePairs(lines, plats)
	cacheMap, err := s.search.LoadQuoteCachesForKeys(ctx, view.BizDate, pairList)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.ItemQuotes, 0, len(lines))
	for _, line := range lines {
		qtyI := bomLineQtyInt(line.Qty)
		var quotes []*v1.PlatformQuote
		mergeKey := biz.NormalizeMPNForBOMSearch(line.Mpn)
		for _, pid := range plats {
			pid = biz.NormalizePlatformID(pid)
			snap := cacheMap[quoteCachePairKey(mergeKey, pid)]
			if snap == nil || !quoteCacheUsable(snap) {
				continue
			}
			var rows []biz.AgentQuoteRow
			if err := json.Unmarshal(snap.QuotesJSON, &rows); err != nil {
				s.log.Warnf("SearchQuotes: skip corrupt quotes_json session=%s mpn=%s platform=%s: %v", sid, line.Mpn, pid, err)
				continue
			}
			for _, row := range rows {
				quotes = append(quotes, agentRowToPlatformQuote(pid, row, qtyI))
			}
		}
		out = append(out, &v1.ItemQuotes{
			Model:    line.Mpn,
			Quantity: int32(qtyI),
			Quotes:   quotes,
		})
	}
	return &v1.SearchQuotesReply{ItemQuotes: out}, nil
}

func (s *BomService) AutoMatch(ctx context.Context, req *v1.AutoMatchRequest) (*v1.AutoMatchReply, error) {
	ctx = context.WithoutCancel(ctx)
	if !s.matchDepsOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sid, err := parseBomSessionID(req.GetBomId())
	if err != nil {
		return nil, err
	}
	_ = req.GetStrategy()
	view, lines, plats, err := s.loadSessionLinesAndPlatforms(ctx, sid, nil)
	if err != nil {
		return nil, err
	}
	if err := s.matchReadinessError(ctx, sid, view, lines); err != nil {
		return nil, err
	}
	items, total, err := s.computeMatchItems(ctx, view, lines, plats)
	if err != nil {
		return nil, err
	}
	return &v1.AutoMatchReply{Items: items, TotalAmount: total}, nil
}

func (s *BomService) GetBOM(ctx context.Context, req *v1.GetBOMRequest) (*v1.GetBOMReply, error) {
	return nil, notImplemented("GetBOM 未实现，请使用 GetSession / GetBOMLines")
}

func (s *BomService) GetMatchResult(ctx context.Context, req *v1.GetMatchResultRequest) (*v1.GetMatchResultReply, error) {
	if !s.matchDepsOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sid, err := parseBomSessionID(req.GetBomId())
	if err != nil {
		return nil, err
	}
	view, lines, plats, err := s.loadSessionLinesAndPlatforms(ctx, sid, nil)
	if err != nil {
		return nil, err
	}
	if err := s.matchReadinessError(ctx, sid, view, lines); err != nil {
		return nil, err
	}
	items, total, err := s.computeMatchItems(ctx, view, lines, plats)
	if err != nil {
		return nil, err
	}
	return &v1.GetMatchResultReply{Items: items, TotalAmount: total}, nil
}

func parseBomSessionID(bomID string) (string, error) {
	id := strings.TrimSpace(bomID)
	if id == "" {
		return "", kerrors.BadRequest("BAD_BOM_ID", "bom_id required")
	}
	if _, err := uuid.Parse(id); err != nil {
		return "", kerrors.BadRequest("BAD_BOM_ID", "bom_id must be a valid session UUID")
	}
	return id, nil
}

func (s *BomService) loadSessionLinesAndPlatforms(ctx context.Context, sid string, reqPlats []string) (*biz.BOMSessionView, []data.BomSessionLine, []string, error) {
	view, err := s.session.GetSession(ctx, sid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, kerrors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		return nil, nil, nil, err
	}
	lines, err := s.dataListLines(ctx, sid)
	if err != nil {
		return nil, nil, nil, err
	}
	var plats []string
	if len(reqPlats) == 0 {
		plats = append([]string(nil), view.PlatformIDs...)
	} else {
		for _, p := range reqPlats {
			p = strings.TrimSpace(p)
			if p != "" {
				plats = append(plats, biz.NormalizePlatformID(p))
			}
		}
	}
	return view, lines, plats, nil
}

// matchReadinessError 在会话尚未满足与 GetReadiness 一致的「可进入配单」条件时拒绝。
// gRPC FailedPrecondition 常映射为 HTTP 400；此处选用 ServiceUnavailable，表示依赖侧（搜索任务/缓存）未就绪、可稍后重试。
func (s *BomService) matchReadinessError(ctx context.Context, sid string, view *biz.BOMSessionView, lines []data.BomSessionLine) error {
	tasks, err := s.search.ListTasksForSession(ctx, sid)
	if err != nil {
		return err
	}
	lineSnaps := make([]biz.LineReadinessSnapshot, 0, len(lines))
	for _, ln := range lines {
		lineSnaps = append(lineSnaps, biz.LineReadinessSnapshot{MpnNorm: biz.NormalizeMPNForBOMSearch(ln.Mpn)})
	}
	lenient := biz.ReadinessFromTasks(biz.ReadinessLenient, tasks, lineSnaps, view.PlatformIDs)
	strict := biz.ReadinessFromTasks(biz.ReadinessStrict, tasks, lineSnaps, view.PlatformIDs)
	can := false
	switch view.Status {
	case "data_ready":
		can = true
	case "blocked":
		can = false
	default:
		if lenient && strict {
			can = true
		} else if lenient && !strict && strings.TrimSpace(view.ReadinessMode) == biz.ReadinessStrict {
			can = false
		}
	}
	if !can {
		return kerrors.ServiceUnavailable("BOM_NOT_READY", "session data not ready for match; see GetReadiness")
	}
	return nil
}

func quoteCacheUsable(snap *biz.QuoteCacheSnapshot) bool {
	if snap == nil {
		return false
	}
	oc := strings.ToLower(strings.TrimSpace(snap.Outcome))
	if oc == "no_mpn_match" || oc == "no_result" {
		return false
	}
	return len(snap.QuotesJSON) > 0
}

// quoteCacheUnusableReason 说明为何不参与配单（与 quoteCacheUsable 判定一致）；hit=false 时仅返回 miss。
func quoteCacheUnusableReason(hit bool, snap *biz.QuoteCacheSnapshot) string {
	if !hit {
		return "quote_cache_miss"
	}
	if snap == nil {
		return "snapshot_nil"
	}
	oc := strings.ToLower(strings.TrimSpace(snap.Outcome))
	switch oc {
	case "no_mpn_match":
		return "outcome_no_mpn_match"
	case "no_result":
		return "outcome_no_result"
	}
	if len(snap.QuotesJSON) == 0 {
		return "quotes_json_empty"
	}
	return ""
}

func (s *BomService) bomMatchBaseCCY() string {
	if s.bomMatch != nil {
		if v := strings.TrimSpace(s.bomMatch.GetBaseCcy()); v != "" {
			return v
		}
	}
	return "CNY"
}

func (s *BomService) bomMatchRoundingMode() string {
	if s.bomMatch != nil {
		if v := strings.TrimSpace(s.bomMatch.GetRoundingMode()); v != "" {
			return v
		}
	}
	return "decimal6"
}

func (s *BomService) bomMatchParseTiers() bool {
	if s.bomMatch == nil {
		return true
	}
	return s.bomMatch.GetParsePriceTierStrings()
}

func bomLineQtyInt(q *float64) int {
	if q == nil || *q <= 0 {
		return 1
	}
	v := int(math.Round(*q))
	if v < 1 {
		return 1
	}
	return v
}

func moqDigitsPositive(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	start := -1
	for i, r := range s {
		if r >= '0' && r <= '9' {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			v, err := strconv.Atoi(s[start:i])
			if err == nil && v > 0 {
				return v
			}
			start = -1
		}
	}
	if start >= 0 {
		v, err := strconv.Atoi(s[start:])
		if err == nil && v > 0 {
			return v
		}
	}
	return 0
}

func matchSortKeyFromPick(pick biz.LineMatchPick, platformID, roundingMode string) biz.MatchSortKey {
	leadDays := biz.MatchLeadDaysUnknown
	if d, ok := biz.ParseLeadDays(pick.Row.LeadTime, platformID); ok {
		leadDays = d
	}
	stockVal := int64(0)
	if sq, ok := biz.ParseCompareStock(pick.Row.Stock); ok {
		stockVal = sq
	}
	return biz.MatchSortKey{
		UnitPriceBaseQuantized: biz.QuantizeUnitPriceBase(roundingMode, pick.UnitPriceBase),
		LeadDays:               leadDays,
		StockParsed:            stockVal,
		PlatformID:             biz.NormalizePlatformID(platformID),
	}
}

func agentRowToPlatformQuote(platformID string, row biz.AgentQuoteRow, _ int) *v1.PlatformQuote {
	pq := &v1.PlatformQuote{
		Platform:      biz.NormalizePlatformID(platformID),
		MatchedModel:  row.Model,
		Manufacturer:  row.Manufacturer,
		Description:   row.Desc,
		LeadTime:      row.LeadTime,
		PriceTiers:    row.PriceTiers,
		HkPrice:       row.HKPrice,
		MainlandPrice: row.MainlandPrice,
		Package:       row.Package,
	}
	if sq, ok := biz.ParseCompareStock(row.Stock); ok {
		pq.Stock = sq
	}
	if v := moqDigitsPositive(row.MOQ); v > 0 {
		pq.Moq = int32(v)
	}
	return pq
}

func noMatchItem(line data.BomSessionLine, qtyI int, mfrMismatch []string) *v1.MatchItem {
	_ = mfrMismatch
	return &v1.MatchItem{
		Index:              int32(line.LineNo),
		Model:              line.Mpn,
		Quantity:           int32(qtyI),
		MatchStatus:        "no_match",
		DemandManufacturer: derefStrPtr(line.Mfr),
		DemandPackage:      derefStrPtr(line.Package),
	}
}

func matchItemFromPick(line data.BomSessionLine, qtyI int, pick biz.LineMatchPick, platformID string, mfrMismatch []string) *v1.MatchItem {
	_ = mfrMismatch
	subtotal := pick.UnitPriceBase * float64(qtyI)
	var stock int64
	if sq, ok := biz.ParseCompareStock(pick.Row.Stock); ok {
		stock = sq
	}
	return &v1.MatchItem{
		Index:              int32(line.LineNo),
		Model:              line.Mpn,
		Quantity:           int32(qtyI),
		MatchedModel:       pick.Row.Model,
		Manufacturer:       pick.Row.Manufacturer,
		Platform:           biz.NormalizePlatformID(platformID),
		LeadTime:           pick.Row.LeadTime,
		Stock:              stock,
		UnitPrice:          pick.UnitPriceBase,
		Subtotal:           subtotal,
		MatchStatus:        "exact",
		DemandManufacturer: derefStrPtr(line.Mfr),
		DemandPackage:      derefStrPtr(line.Package),
	}
}

func (s *BomService) CreateSession(ctx context.Context, req *v1.CreateSessionRequest) (*v1.CreateSessionReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	var cust, phone, email, extra *string
	if req.GetCustomerName() != "" {
		c := req.GetCustomerName()
		cust = &c
	}
	if req.GetContactPhone() != "" {
		p := req.GetContactPhone()
		phone = &p
	}
	if req.GetContactEmail() != "" {
		e := req.GetContactEmail()
		email = &e
	}
	if req.GetContactExtra() != "" {
		x := req.GetContactExtra()
		extra = &x
	}
	id, bd, rev, err := s.session.CreateSession(ctx, req.GetTitle(), req.GetPlatformIds(), cust, phone, email, extra, nil)
	if err != nil {
		return nil, err
	}
	return &v1.CreateSessionReply{
		SessionId:         id,
		BizDate:           bd.Format("2006-01-02"),
		SelectionRevision: int32(rev),
	}, nil
}

func (s *BomService) GetSession(ctx context.Context, req *v1.GetSessionRequest) (*v1.GetSessionReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	v, err := s.session.GetSession(ctx, req.GetSessionId())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, kerrors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		return nil, err
	}
	return &v1.GetSessionReply{
		SessionId:         v.SessionID,
		Title:             v.Title,
		Status:            v.Status,
		BizDate:           v.BizDate.Format("2006-01-02"),
		SelectionRevision: int32(v.SelectionRevision),
		PlatformIds:       append([]string(nil), v.PlatformIDs...),
		CustomerName:      v.CustomerName,
		ContactPhone:      v.ContactPhone,
		ContactEmail:      v.ContactEmail,
		ContactExtra:      v.ContactExtra,
	}, nil
}

func (s *BomService) ListSessions(ctx context.Context, req *v1.ListSessionsRequest) (*v1.ListSessionsReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	items, total, err := s.session.ListSessions(ctx, req.GetPage(), req.GetPageSize(), req.GetStatus(), req.GetBizDate(), req.GetQ())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.SessionListItem, 0, len(items))
	for _, it := range items {
		out = append(out, &v1.SessionListItem{
			SessionId:    it.SessionID,
			Title:        it.Title,
			CustomerName: it.CustomerName,
			Status:       it.Status,
			BizDate:      it.BizDate,
			UpdatedAt:    it.UpdatedAt,
			LineCount:    it.LineCount,
		})
	}
	return &v1.ListSessionsReply{Items: out, Total: total}, nil
}

func (s *BomService) PatchSession(ctx context.Context, req *v1.PatchSessionRequest) (*v1.GetSessionReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	var title, cust, phone, email, extra *string
	if req.Title != nil {
		title = req.Title
	}
	if req.CustomerName != nil {
		cust = req.CustomerName
	}
	if req.ContactPhone != nil {
		phone = req.ContactPhone
	}
	if req.ContactEmail != nil {
		email = req.ContactEmail
	}
	if req.ContactExtra != nil {
		extra = req.ContactExtra
	}
	if err := s.session.PatchSession(ctx, req.GetSessionId(), title, cust, phone, email, extra, nil); err != nil {
		return nil, err
	}
	return s.GetSession(ctx, &v1.GetSessionRequest{SessionId: req.GetSessionId()})
}

func (s *BomService) PutPlatforms(ctx context.Context, req *v1.PutPlatformsRequest) (*v1.PutPlatformsReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	rev, err := s.session.PutPlatforms(ctx, req.GetSessionId(), req.GetPlatformIds(), req.GetExpectedRevision())
	if err != nil {
		if errors.Is(err, biz.ErrBOMSessionRevisionMismatch) {
			return nil, kerrors.Conflict("REVISION_MISMATCH", err.Error())
		}
		return nil, err
	}
	return &v1.PutPlatformsReply{SelectionRevision: int32(rev)}, nil
}

func (s *BomService) GetReadiness(ctx context.Context, req *v1.GetReadinessRequest) (*v1.GetReadinessReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sid := req.GetSessionId()
	view, err := s.session.GetSession(ctx, sid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, kerrors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		return nil, err
	}
	lines, err := s.session.ListSessionLines(ctx, sid)
	if err != nil {
		return nil, err
	}
	tasks, err := s.search.ListTasksForSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	lineSnaps := make([]biz.LineReadinessSnapshot, 0, len(lines))
	for _, ln := range lines {
		lineSnaps = append(lineSnaps, biz.LineReadinessSnapshot{MpnNorm: biz.NormalizeMPNForBOMSearch(ln.Mpn)})
	}
	lenient := biz.ReadinessFromTasks(biz.ReadinessLenient, tasks, lineSnaps, view.PlatformIDs)
	strict := biz.ReadinessFromTasks(biz.ReadinessStrict, tasks, lineSnaps, view.PlatformIDs)
	phase := "searching"
	can := false
	block := ""
	switch view.Status {
	case "data_ready":
		phase = "data_ready"
		can = true
	case "blocked":
		phase = "blocked"
		block = "strict_mode_no_quote_per_line"
	default:
		if lenient && strict {
			phase = "data_ready"
			can = true
		} else if lenient && !strict && strings.TrimSpace(view.ReadinessMode) == biz.ReadinessStrict {
			phase = "blocked"
			block = "strict_mode_no_quote_per_line"
		}
	}
	return &v1.GetReadinessReply{
		SessionId:         sid,
		BizDate:           view.BizDate.Format("2006-01-02"),
		SelectionRevision: int32(view.SelectionRevision),
		Phase:             phase,
		CanEnterMatch:     can,
		BlockReason:       block,
	}, nil
}

func (s *BomService) GetBOMLines(ctx context.Context, req *v1.GetBOMLinesRequest) (*v1.GetBOMLinesReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	rows, err := s.dataListLines(ctx, req.GetSessionId())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.BOMLineRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, &v1.BOMLineRow{
			LineId:  strconv.FormatInt(row.ID, 10),
			LineNo:  int32(row.LineNo),
			Mpn:     row.Mpn,
			Mfr:     derefStrPtr(row.Mfr),
			Package: derefStrPtr(row.Package),
			Qty:     derefFloat(row.Qty),
		})
	}
	return &v1.GetBOMLinesReply{Lines: out}, nil
}

func (s *BomService) dataListLines(ctx context.Context, sessionID string) ([]data.BomSessionLine, error) {
	type lineLister interface {
		ListSessionLinesFull(ctx context.Context, sessionID string) ([]data.BomSessionLine, error)
	}
	if sl, ok := s.session.(lineLister); ok {
		return sl.ListSessionLinesFull(ctx, sessionID)
	}
	v, err := s.session.ListSessionLines(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]data.BomSessionLine, 0, len(v))
	for _, x := range v {
		out = append(out, data.BomSessionLine{ID: x.ID, LineNo: x.LineNo, Mpn: x.Mpn})
	}
	return out, nil
}

func derefStrPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefFloat(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func (s *BomService) GetSessionSearchTaskCoverage(ctx context.Context, req *v1.GetSessionSearchTaskCoverageRequest) (*v1.GetSessionSearchTaskCoverageReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sid := req.GetSessionId()
	view, err := s.session.GetSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	lines, err := s.session.ListSessionLines(ctx, sid)
	if err != nil {
		return nil, err
	}
	tasks, err := s.search.ListTasksForSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	expected := len(lines) * len(view.PlatformIDs)
	have := make(map[string]struct{})
	for _, t := range tasks {
		k := t.MpnNorm + "\x00" + t.PlatformID
		have[k] = struct{}{}
	}
	var missing []*v1.SearchTaskMissingItem
	for _, ln := range lines {
		mn := biz.NormalizeMPNForBOMSearch(ln.Mpn)
		for _, pid := range view.PlatformIDs {
			pid = biz.NormalizePlatformID(pid)
			k := mn + "\x00" + pid
			if _, ok := have[k]; !ok {
				missing = append(missing, &v1.SearchTaskMissingItem{
					LineId:     strconv.FormatInt(ln.ID, 10),
					LineNo:     int32(ln.LineNo),
					MpnNorm:    mn,
					PlatformId: pid,
					Reason:     "missing_task_row",
				})
			}
		}
	}
	consistent := len(missing) == 0
	return &v1.GetSessionSearchTaskCoverageReply{
		Consistent:        consistent,
		ExpectedTaskCount: int32(expected),
		ExistingTaskCount: int32(len(tasks)),
		MissingTasks:      missing,
	}, nil
}

func (s *BomService) ListSessionSearchTasks(ctx context.Context, req *v1.ListSessionSearchTasksRequest) (*v1.ListSessionSearchTasksReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sid := strings.TrimSpace(req.GetSessionId())
	view, err := s.session.GetSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	lines, err := s.session.ListSessionLines(ctx, sid)
	if err != nil {
		return nil, err
	}
	existing, err := s.search.ListSearchTaskStatusRows(ctx, sid)
	if err != nil {
		return nil, err
	}

	byKey := make(map[string]biz.SearchTaskStatusRow, len(existing))
	for _, row := range existing {
		byKey[searchTaskStatusKey(row.MpnNorm, row.PlatformID)] = row
	}

	rows := make([]biz.SearchTaskStatusRow, 0, len(lines)*len(view.PlatformIDs))
	for _, line := range lines {
		mpnNorm := biz.NormalizeMPNForBOMSearch(line.Mpn)
		for _, platformRaw := range view.PlatformIDs {
			platformID := biz.NormalizePlatformID(platformRaw)
			row, ok := byKey[searchTaskStatusKey(mpnNorm, platformID)]
			if !ok {
				row = biz.SearchTaskStatusRow{SearchTaskState: biz.SearchTaskUIStateMissing}
			}
			row.LineID = uint64(line.ID)
			row.LineNo = line.LineNo
			row.MpnRaw = line.Mpn
			row.MpnNorm = mpnNorm
			row.PlatformID = platformID
			row.SearchTaskState = biz.NormalizeBOMSearchTaskState(row.SearchTaskState)
			row.SearchUIState = biz.MapBOMSearchTaskUIState(row.SearchTaskState)
			row.Retryable, row.RetryBlockedReason = biz.CanRetryBOMSearchTask(row.SearchTaskState, biz.SearchTaskRetrySingleManual)
			rows = append(rows, row)
		}
	}

	summary := biz.BuildSearchTaskStatusSummary(rows)
	out := &v1.ListSessionSearchTasksReply{
		SessionId: sid,
		Summary: &v1.SearchTaskStatusSummary{
			Total:     int32(summary.Total),
			Pending:   int32(summary.Pending),
			Searching: int32(summary.Searching),
			Succeeded: int32(summary.Succeeded),
			NoData:    int32(summary.NoData),
			Failed:    int32(summary.Failed),
			Skipped:   int32(summary.Skipped),
			Cancelled: int32(summary.Cancelled),
			Missing:   int32(summary.Missing),
			Retryable: int32(summary.Retryable),
		},
		Tasks: make([]*v1.SessionSearchTaskRow, 0, len(rows)),
	}
	for _, row := range rows {
		out.Tasks = append(out.Tasks, searchTaskStatusRowToProto(row))
	}
	return out, nil
}

func searchTaskStatusKey(mpnNorm, platformID string) string {
	return biz.NormalizeMPNForBOMSearch(mpnNorm) + "\x00" + biz.NormalizePlatformID(platformID)
}

func searchTaskStatusRowToProto(row biz.SearchTaskStatusRow) *v1.SessionSearchTaskRow {
	return &v1.SessionSearchTaskRow{
		LineId:             strconv.FormatUint(row.LineID, 10),
		LineNo:             int32(row.LineNo),
		MpnRaw:             row.MpnRaw,
		MpnNorm:            row.MpnNorm,
		PlatformId:         row.PlatformID,
		PlatformName:       row.PlatformName,
		SearchTaskId:       strconv.FormatUint(row.SearchTaskID, 10),
		SearchTaskState:    row.SearchTaskState,
		SearchUiState:      row.SearchUIState,
		Retryable:          row.Retryable,
		RetryBlockedReason: row.RetryBlockedReason,
		DispatchTaskId:     row.DispatchTaskID,
		DispatchTaskState:  row.DispatchTaskState,
		DispatchAgentId:    row.DispatchAgentID,
		DispatchResult:     row.DispatchResult,
		LeaseDeadlineAt:    formatOptionalTime(row.LeaseDeadlineAt),
		Attempt:            int32(row.Attempt),
		RetryMax:           int32(row.RetryMax),
		UpdatedAt:          formatOptionalTime(row.UpdatedAt),
		LastError:          row.LastError,
	}
}

func formatOptionalTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func (s *BomService) CreateSessionLine(ctx context.Context, req *v1.CreateSessionLineRequest) (*v1.CreateSessionLineReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	var qty *float64
	if req.GetQty() != 0 {
		q := req.GetQty()
		qty = &q
	}
	var raw, extra *string
	if req.GetRaw() != "" {
		r := req.GetRaw()
		raw = &r
	}
	if req.GetExtraJson() != "" {
		e := req.GetExtraJson()
		extra = &e
	}
	id, lineNo, rev, err := s.session.CreateSessionLine(ctx, req.GetSessionId(), req.GetMpn(), req.GetMfr(), req.GetPackage(), qty, raw, extra)
	if err != nil {
		return nil, err
	}
	view, err := s.session.GetSession(ctx, req.GetSessionId())
	if err != nil {
		return nil, err
	}
	var pairs []biz.MpnPlatformPair
	mn := biz.NormalizeMPNForBOMSearch(req.GetMpn())
	for _, p := range view.PlatformIDs {
		pairs = append(pairs, biz.MpnPlatformPair{MpnNorm: mn, PlatformID: biz.NormalizePlatformID(p)})
	}
	if err := s.search.UpsertPendingTasks(ctx, req.GetSessionId(), view.BizDate, rev, pairs); err != nil {
		return nil, err
	}
	s.tryMergeDispatchSession(ctx, req.GetSessionId())
	return &v1.CreateSessionLineReply{LineId: strconv.FormatInt(id, 10), LineNo: lineNo}, nil
}

func (s *BomService) PatchSessionLine(ctx context.Context, req *v1.PatchSessionLineRequest) (*v1.PatchSessionLineReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sid := req.GetSessionId()
	lid, err := strconv.ParseInt(req.GetLineId(), 10, 64)
	if err != nil {
		return nil, kerrors.BadRequest("BAD_LINE_ID", "invalid line_id")
	}
	linesBefore, err := s.dataListLines(ctx, sid)
	if err != nil {
		return nil, err
	}
	var oldMpn string
	for _, ln := range linesBefore {
		if ln.ID == lid {
			oldMpn = ln.Mpn
			break
		}
	}
	var mpn, mfr, pkg, raw, extra *string
	var qty *float64
	if req.Mpn != nil {
		mpn = req.Mpn
	}
	if req.Mfr != nil {
		mfr = req.Mfr
	}
	if req.Package != nil {
		pkg = req.Package
	}
	if req.Qty != nil {
		q := req.GetQty()
		qty = &q
	}
	if req.Raw != nil {
		raw = req.Raw
	}
	if req.ExtraJson != nil {
		extra = req.ExtraJson
	}
	rev, err := s.session.UpdateSessionLine(ctx, sid, lid, mpn, mfr, pkg, qty, raw, extra)
	if err != nil {
		return nil, err
	}
	view, err := s.session.GetSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	newMpn := oldMpn
	if mpn != nil {
		newMpn = *mpn
	}
	if biz.NormalizeMPNForBOMSearch(oldMpn) != biz.NormalizeMPNForBOMSearch(newMpn) {
		_ = s.search.CancelTasksBySessionMpnNorm(ctx, sid, oldMpn)
		var pairs []biz.MpnPlatformPair
		mn := biz.NormalizeMPNForBOMSearch(newMpn)
		for _, p := range view.PlatformIDs {
			pairs = append(pairs, biz.MpnPlatformPair{MpnNorm: mn, PlatformID: biz.NormalizePlatformID(p)})
		}
		if err := s.search.UpsertPendingTasks(ctx, sid, view.BizDate, rev, pairs); err != nil {
			return nil, err
		}
		s.tryMergeDispatchSession(ctx, sid)
	}
	var lineNo int32
	for _, ln := range linesBefore {
		if ln.ID == lid {
			lineNo = int32(ln.LineNo)
			break
		}
	}
	return &v1.PatchSessionLineReply{LineId: req.GetLineId(), LineNo: lineNo}, nil
}

func (s *BomService) DeleteSessionLine(ctx context.Context, req *v1.DeleteSessionLineRequest) (*v1.DeleteSessionLineReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sid := req.GetSessionId()
	lid, err := strconv.ParseInt(req.GetLineId(), 10, 64)
	if err != nil {
		return nil, kerrors.BadRequest("BAD_LINE_ID", "invalid line_id")
	}
	lines, err := s.dataListLines(ctx, sid)
	if err != nil {
		return nil, err
	}
	for _, ln := range lines {
		if ln.ID == lid {
			_ = s.search.CancelTasksBySessionMpnNorm(ctx, sid, ln.Mpn)
			break
		}
	}
	if err := s.session.DeleteSessionLine(ctx, sid, lid); err != nil {
		return nil, err
	}
	return &v1.DeleteSessionLineReply{}, nil
}

func (s *BomService) RetrySearchTasks(ctx context.Context, req *v1.RetrySearchTasksRequest) (*v1.RetrySearchTasksReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sid := req.GetSessionId()
	view, err := s.session.GetSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	var n int32
	for _, it := range req.GetItems() {
		mn := biz.NormalizeMPNForBOMSearch(it.GetMpn())
		pid := biz.NormalizePlatformID(it.GetPlatformId())
		cur, err := s.search.GetTaskStateBySessionKey(ctx, sid, mn, pid, view.BizDate)
		if err != nil {
			continue
		}
		if cur == "" {
			if err := s.search.UpsertPendingTasks(ctx, sid, view.BizDate, view.SelectionRevision, []biz.MpnPlatformPair{{MpnNorm: mn, PlatformID: pid}}); err != nil {
				continue
			}
			n++
			continue
		}
		to, terr := biz.BomSearchTaskTransition(cur, "retry_backoff")
		if terr != nil {
			if cur == "failed_terminal" || cur == "skipped" {
				to = "pending"
			} else {
				continue
			}
		}
		if err := s.search.UpdateTaskStateBySessionKey(ctx, sid, mn, pid, view.BizDate, to); err != nil {
			continue
		}
		n++
	}
	if n > 0 {
		s.tryMergeDispatchSession(ctx, sid)
	}
	return &v1.RetrySearchTasksReply{Accepted: n}, nil
}

func mapProtoSearchStatus(st string) (fsmEvent string, terminalState string) {
	switch strings.ToLower(strings.TrimSpace(st)) {
	case "succeeded_quotes":
		return "result_ok_with_quotes", "succeeded"
	case "succeeded_no_mpn":
		return "result_ok_empty", "no_result"
	case "failed":
		return "error_terminal", "failed_terminal"
	default:
		return "", ""
	}
}

func (s *BomService) SubmitBomSearchResult(ctx context.Context, req *v1.SubmitBomSearchResultRequest) (*v1.SubmitBomSearchResultReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	sid := strings.TrimSpace(req.GetSessionId())
	view, err := s.session.GetSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	mpn := biz.NormalizeMPNForBOMSearch(req.GetMpnNorm())
	pid := biz.NormalizePlatformID(req.GetPlatformId())
	ev, direct := mapProtoSearchStatus(req.GetStatus())
	if ev == "" {
		return nil, kerrors.BadRequest("BAD_STATUS", "unknown status")
	}
	var last *string
	if msg := strings.TrimSpace(req.GetErrorMessage()); msg != "" {
		last = &msg
	}
	qj := []byte(req.GetQuotesJson())
	nd := []byte(req.GetNoMpnDetailJson())
	cid := strings.TrimSpace(req.GetCaichipTaskId())

	if cid != "" {
		lookups, err := s.search.ListSearchTaskLookupsByCaichipTaskID(ctx, cid)
		if err != nil {
			return nil, err
		}
		if len(lookups) > 0 {
			sessions := make(map[string]struct{})
			for _, L := range lookups {
				cur, err := s.search.GetTaskStateBySessionKey(ctx, L.SessionID, L.MpnNorm, L.PlatformID, L.BizDate)
				if err != nil {
					return nil, err
				}
				to, terr := biz.BomSearchTaskTransition(cur, ev)
				if terr != nil || cur == "" {
					to = direct
				}
				switch to {
				case "succeeded":
					err = s.search.FinalizeSearchTask(ctx, L.SessionID, L.MpnNorm, L.PlatformID, L.BizDate, cid, to, last, "ok", qj, nd)
				case "no_result":
					err = s.search.FinalizeSearchTask(ctx, L.SessionID, L.MpnNorm, L.PlatformID, L.BizDate, cid, to, last, "", nil, nd)
				default:
					err = s.search.FinalizeSearchTask(ctx, L.SessionID, L.MpnNorm, L.PlatformID, L.BizDate, cid, to, last, "", nil, nil)
				}
				if err != nil {
					if errors.Is(err, data.ErrSearchTaskNotFound) {
						return nil, kerrors.NotFound("TASK_NOT_FOUND", "bom_search_task not found")
					}
					return nil, err
				}
				sessions[L.SessionID] = struct{}{}
			}
			for sID := range sessions {
				_ = biz.TryMarkSessionDataReady(ctx, s.session, s.search, sID)
			}
			return &v1.SubmitBomSearchResultReply{
				Accepted:   true,
				ServerTime: time.Now().Format(time.RFC3339Nano),
			}, nil
		}
	}

	cur, err := s.search.GetTaskStateBySessionKey(ctx, sid, mpn, pid, view.BizDate)
	if err != nil {
		return nil, err
	}
	to, terr := biz.BomSearchTaskTransition(cur, ev)
	if terr != nil || cur == "" {
		to = direct
	}

	switch to {
	case "succeeded":
		if err := s.search.FinalizeSearchTask(ctx, sid, mpn, pid, view.BizDate, cid, to, last, "ok", qj, nd); err != nil {
			if errors.Is(err, data.ErrSearchTaskNotFound) {
				return nil, kerrors.NotFound("TASK_NOT_FOUND", "bom_search_task not found")
			}
			return nil, err
		}
	case "no_result":
		if err := s.search.FinalizeSearchTask(ctx, sid, mpn, pid, view.BizDate, cid, to, last, "", nil, nd); err != nil {
			if errors.Is(err, data.ErrSearchTaskNotFound) {
				return nil, kerrors.NotFound("TASK_NOT_FOUND", "bom_search_task not found")
			}
			return nil, err
		}
	default:
		if err := s.search.FinalizeSearchTask(ctx, sid, mpn, pid, view.BizDate, cid, to, last, "", nil, nil); err != nil {
			if errors.Is(err, data.ErrSearchTaskNotFound) {
				return nil, kerrors.NotFound("TASK_NOT_FOUND", "bom_search_task not found")
			}
			return nil, err
		}
	}
	_ = biz.TryMarkSessionDataReady(ctx, s.session, s.search, sid)
	return &v1.SubmitBomSearchResultReply{
		Accepted:   true,
		ServerTime: time.Now().Format(time.RFC3339Nano),
	}, nil
}

func (s *BomService) UploadBOM(ctx context.Context, req *v1.UploadBOMRequest) (*v1.UploadBOMReply, error) {
	sid := strings.TrimSpace(req.GetSessionId())
	if sid == "" {
		return nil, notImplemented("请使用 session_id 将 Excel 导入到 bom_session_line")
	}
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	parseModeRaw := strings.TrimSpace(req.GetParseMode())
	pmLower := strings.ToLower(parseModeRaw)
	var lines []biz.BomImportLine
	var ierrs []biz.BomImportError
	switch pmLower {
	case "llm":
		if s.openai == nil {
			return nil, kerrors.BadRequest("BOM_LLM_DISABLED", "parse_mode=llm 需要在配置中设置 openai.api_key")
		}
		rows, ferrs := biz.ReadBomImportFirstSheetFromReader(bytes.NewReader(req.GetFile()))
		if len(ferrs) > 0 {
			return nil, kerrors.BadRequest("BOM_IMPORT", ferrs[0].Error())
		}
		if len(rows) > biz.MaxBomLLMSheetRows {
			return nil, kerrors.BadRequest("BOM_LLM", fmt.Sprintf("工作表行数超过 llm 模式上限 %d，请拆分文件", biz.MaxBomLLMSheetRows))
		}
		user := biz.BuildBomLLMUserPrompt(rows)
		if len(user) > biz.MaxBomLLMPromptBytes {
			return nil, kerrors.BadRequest("BOM_LLM", fmt.Sprintf("工作表体积超过 llm 模式上限（约 %d 字节），请拆分或删减列", biz.MaxBomLLMPromptBytes))
		}
		raw, err := s.openai.Chat(context.WithoutCancel(ctx), biz.BomLLMSystemPrompt(), user)
		if err != nil {
			return nil, kerrors.BadRequest("BOM_LLM", err.Error())
		}
		lines, ierrs = biz.ParseBomImportLinesFromLLMJSON(raw)
	default:
		r := bytes.NewReader(req.GetFile())
		lines, ierrs = biz.ParseBomImportRowsWithColumnMapping(r, false, req.GetColumnMapping())
	}
	if len(ierrs) > 0 {
		return nil, kerrors.BadRequest("BOM_IMPORT", ierrs[0].Error())
	}
	var pmPtr *string
	if parseModeRaw != "" {
		pmPtr = &parseModeRaw
	}
	if _, err := s.session.ReplaceSessionLines(ctx, sid, lines, pmPtr); err != nil {
		return nil, err
	}
	if err := s.search.CancelAllTasksBySession(ctx, sid); err != nil {
		return nil, err
	}
	view, err := s.session.GetSession(ctx, sid)
	if err != nil {
		return nil, err
	}
	pairs := buildMpnPlatformPairs(lines, view.PlatformIDs)
	if err := s.search.UpsertPendingTasks(ctx, sid, view.BizDate, view.SelectionRevision, pairs); err != nil {
		return nil, err
	}
	s.tryMergeDispatchSession(ctx, sid)
	items := make([]*v1.ParsedItem, 0, len(lines))
	for i, ln := range lines {
		var q int32
		if ln.Qty != nil {
			q = int32(*ln.Qty)
		}
		items = append(items, &v1.ParsedItem{
			Index:        int32(i + 1),
			Model:        ln.Mpn,
			Manufacturer: ln.Mfr,
			Package:      ln.Package,
			Quantity:     q,
		})
	}
	return &v1.UploadBOMReply{
		BomId: sid,
		Items: items,
		Total: int32(len(items)),
	}, nil
}

func buildMpnPlatformPairs(lines []biz.BomImportLine, platforms []string) []biz.MpnPlatformPair {
	var pairs []biz.MpnPlatformPair
	for _, ln := range lines {
		mn := biz.NormalizeMPNForBOMSearch(ln.Mpn)
		for _, p := range platforms {
			pairs = append(pairs, biz.MpnPlatformPair{MpnNorm: mn, PlatformID: biz.NormalizePlatformID(p)})
		}
	}
	return pairs
}

func normalizeSessionPlatforms(ids []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, p := range ids {
		n := biz.NormalizePlatformID(p)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

func (s *BomService) DownloadTemplate(ctx context.Context, req *v1.DownloadTemplateRequest) (*v1.DownloadTemplateReply, error) {
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetSheetRow(sheet, "A1", &[]any{"型号", "数量", "厂牌", "封装", "参数"})
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return &v1.DownloadTemplateReply{File: buf.Bytes(), Filename: "bom_template.xlsx"}, nil
}

func (s *BomService) ExportSession(ctx context.Context, req *v1.ExportSessionRequest) (*v1.ExportSessionReply, error) {
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	rows, err := s.dataListLines(ctx, req.GetSessionId())
	if err != nil {
		return nil, err
	}
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetSheetRow(sheet, "A1", &[]any{"行号", "型号", "厂牌", "封装", "数量"})
	for i, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		qty := 0.0
		if row.Qty != nil {
			qty = *row.Qty
		}
		_ = f.SetSheetRow(sheet, cell, &[]any{
			row.LineNo,
			row.Mpn,
			derefStrPtr(row.Mfr),
			derefStrPtr(row.Package),
			qty,
		})
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	fn := "bom_export.xlsx"
	if strings.EqualFold(strings.TrimSpace(req.GetFormat()), "csv") {
		fn = "bom_export.csv"
	}
	return &v1.ExportSessionReply{File: buf.Bytes(), Filename: fn}, nil
}
