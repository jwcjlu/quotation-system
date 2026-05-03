package service

import (
	"context"
	"encoding/json"
	"errors"
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

// BomService implements the BOM HTTP API.
type BomService struct {
	session     biz.BOMSessionRepo
	search      biz.BOMSearchTaskRepo
	gaps        biz.BOMLineGapRepo
	matchRuns   biz.BOMMatchRunRepo
	merge       biz.MergeDispatchExecutor
	openai      *data.OpenAIChat
	llmChatFn   func(ctx context.Context, system, user string) (string, error)
	fx          *data.BomFxRateRepo
	alias       biz.BomManufacturerAliasRepo
	mfrCleaning biz.BomManufacturerCleaningRepo
	hsMapping   biz.HsModelMappingRepo
	hsItem      biz.HsItemReadRepo
	hsTaxDaily  biz.HsTaxRateDailyRepo
	hsTaxAPI    biz.TaxRateAPIFetcher
	bomMatch    *conf.BomMatch
	log         *log.Helper
}

func (s *BomService) SetManufacturerCleaningRepo(repo biz.BomManufacturerCleaningRepo) *BomService {
	s.mfrCleaning = repo
	return s
}

// NewBomService ...
func NewBomService(
	session biz.BOMSessionRepo,
	search biz.BOMSearchTaskRepo,
	gaps biz.BOMLineGapRepo,
	matchRuns biz.BOMMatchRunRepo,
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
		gaps:       gaps,
		matchRuns:  matchRuns,
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

// matchDepsOK reports whether quote matching dependencies are available.
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
	if err := s.matchReadinessError(ctx, sid, view, lines, false); err != nil {
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
	if err := s.matchReadinessError(ctx, sid, view, lines, true); err != nil {
		return nil, err
	}
	if pending := demandManufacturerCleaningRequired(lines); len(pending) > 0 {
		return nil, kerrors.BadRequest("MFR_CLEANING_REQUIRED", "manufacturer cleaning required for lines: "+formatLineNos(pending))
	}
	items, total, err := s.computeMatchItems(ctx, view, lines, plats)
	if err != nil {
		return nil, err
	}
	return &v1.AutoMatchReply{Items: items, TotalAmount: total}, nil
}

func (s *BomService) GetBOM(ctx context.Context, req *v1.GetBOMRequest) (*v1.GetBOMReply, error) {
	return nil, notImplemented("GetBOM 闂佸搫鐗滄禍婊堟偪閸曨垱鍋濋悽顖ｅ枤缁€澶愭偣閸ヮ亝鑵瑰┑鐐叉喘閹?GetSession / GetBOMLines")
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
	if err := s.matchReadinessError(ctx, sid, view, lines, false); err != nil {
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

// matchReadinessError 闂侀潻璐熼崝瀣閹殿喗瀚氭繝闈涙濮ｏ綁鏌￠崼顐＄凹鐎殿啫鍛儱闁稿繐鎳愰悷?GetReadiness 婵炴垶鎸撮崑鎾绘煠瀹勯偊鍤熸繛鍫熷灴婵″浠﹂挊澶庮唹闁哄鏅滅粙鎴﹀矗閸℃稒鐓€鐎广儱鎳庣粈瀣煏閸℃校婵炵⒈鍨辩粋鎺楁嚋閸偒妲梺褰掓敱鐢偟鍒掗悜钘壩?// gRPC FailedPrecondition 闁汇埄鍨伴幉鈥澄ｈ娴滄悂宕熼鍥╊槹 HTTP 400闂佹寧绋掔粙鎺楊敆濠靛洤绶為柛鏇ㄥ灙閸嬫挻寰勭仦鐐 ServiceUnavailable闂佹寧绋戦惌渚€濡撮崘顏嗙焼閺夌偞澹嗙拹鈺呮偣瑜庨悧鏃堝疾閸洘鏅柛顐ゅ枑閸嬫繄绱掓鏍ㄧ窔闁瑰箍鍨藉畷?缂傚倸鍊归幐鎼佹偤閵娾晜鏅璺侯儐瀵捇鎮樿箛姘惈闁告閰ｆ俊瀛樻媴缁嬭儻顔夌紓浣割儏缁夋挳骞冨Δ鍛厒鐎广儱鐗忓Σ鎼佹煏?
func (s *BomService) matchReadinessError(ctx context.Context, sid string, view *biz.BOMSessionView, lines []data.BomSessionLine, allowNoMatchAfterFilter bool) error {
	if strings.EqualFold(strings.TrimSpace(view.ImportStatus), biz.BOMImportStatusParsing) {
		return kerrors.ServiceUnavailable("BOM_NOT_READY", "session import is parsing; please retry later")
	}
	_, availabilitySummary, err := s.computeLineAvailability(ctx, view, lines, view.PlatformIDs)
	if err != nil {
		return err
	}
	hasHardGap := availabilitySummary.NoDataLineCount+availabilitySummary.CollectionUnavailableLineCount > 0
	hasStrictGap := availabilitySummary.HasStrictBlockingGap()
	if (allowNoMatchAfterFilter && hasHardGap) || (!allowNoMatchAfterFilter && hasStrictGap) {
		return kerrors.ServiceUnavailable("BOM_LINE_AVAILABILITY_GAP", "session has BOM lines without usable data; see GetReadiness")
	}
	// AutoMatch：仅拦「硬缺口」（无数据 / 采集不可用），不再做任务级 BOM_NOT_READY 门禁，
	// 避免与 GetReadiness 展示不一致、或 status 未标 data_ready 时已可出配单结果的场景。
	if allowNoMatchAfterFilter {
		return nil
	}
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

// quoteCacheUnusableReason 闁荤姴娲ら悺銊ノｉ幋鐐碘枖閺夌偞澹嗙粔鍨槈閹惧磭孝鐎殿噮鍓氱粙澶嬫償閵娿儱璧嬮梺鍛婎殕濞测晝妲愬▎鎰枖?quoteCacheUsable 闂佸憡甯囬崐鏇㈡偩閻愵剛鈻旈柍褜鍓熼幊娑欐綇閸撗咁槴闂佹寧绋掑▔顣弔=false 闂佸搫鍟﹢鍦垝鎼淬垺浜ら柡鍌涘缁€鈧?miss闂?
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

func noMatchItem(line data.BomSessionLine, qtyI int, mfrMismatch []string, matchedBy, matchedQueryMpn string) *v1.MatchItem {
	_ = mfrMismatch
	return &v1.MatchItem{
		Index:              int32(line.LineNo),
		Model:              line.Mpn,
		Quantity:           int32(qtyI),
		MatchStatus:        "no_match",
		MatchedBy:          matchedBy,
		MatchedQueryMpn:    matchedQueryMpn,
		DemandManufacturer: derefStrPtr(line.Mfr),
		DemandPackage:      derefStrPtr(line.Package),
	}
}

func matchItemFromPick(line data.BomSessionLine, qtyI int, pick biz.LineMatchPick, platformID string, mfrMismatch []string, matchedBy, matchedQueryMpn string) *v1.MatchItem {
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
		MatchedBy:          matchedBy,
		MatchedQueryMpn:    matchedQueryMpn,
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
		ImportStatus:      v.ImportStatus,
		ImportProgress:    int32(v.ImportProgress),
		ImportStage:       v.ImportStage,
		ImportMessage:     v.ImportMessage,
		ImportErrorCode:   v.ImportErrorCode,
		ImportError:       v.ImportError,
		ImportUpdatedAt:   formatOptTimeRFC3339(v.ImportUpdatedAt),
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
	if view == nil {
		return nil, kerrors.NotFound("SESSION_NOT_FOUND", "session not found")
	}
	lines, err := s.dataListLines(ctx, sid)
	if err != nil {
		return nil, err
	}
	_, availabilitySummary, err := s.computeLineAvailability(ctx, view, lines, view.PlatformIDs)
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
		SessionId:                      sid,
		BizDate:                        view.BizDate.Format("2006-01-02"),
		SelectionRevision:              int32(view.SelectionRevision),
		Phase:                          phase,
		CanEnterMatch:                  can,
		BlockReason:                    block,
		LineTotal:                      int32(availabilitySummary.LineTotal),
		ReadyLineCount:                 int32(availabilitySummary.ReadyLineCount),
		GapLineCount:                   int32(availabilitySummary.GapLineCount),
		NoDataLineCount:                int32(availabilitySummary.NoDataLineCount),
		CollectionUnavailableLineCount: int32(availabilitySummary.CollectionUnavailableLineCount),
		NoMatchAfterFilterLineCount:    int32(availabilitySummary.NoMatchAfterFilterLineCount),
		CollectingLineCount:            int32(availabilitySummary.CollectingLineCount),
		HasStrictBlockingGap:           availabilitySummary.HasStrictBlockingGap(),
	}, nil
}

func (s *BomService) GetBOMLines(ctx context.Context, req *v1.GetBOMLinesRequest) (*v1.GetBOMLinesReply, error) {
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
	if view == nil {
		return nil, kerrors.NotFound("SESSION_NOT_FOUND", "session not found")
	}
	rows, err := s.dataListLines(ctx, sid)
	if err != nil {
		return nil, err
	}
	availability, _, err := s.computeLineAvailability(ctx, view, rows, view.PlatformIDs)
	if err != nil {
		return nil, err
	}
	availabilityByLineNo := make(map[int]biz.LineAvailability, len(availability))
	for _, item := range availability {
		availabilityByLineNo[item.LineNo] = item
	}
	out := make([]*v1.BOMLineRow, 0, len(rows))
	for _, row := range rows {
		lineAvailability := availabilityByLineNo[row.LineNo]
		out = append(out, &v1.BOMLineRow{
			LineId:                   strconv.FormatInt(row.ID, 10),
			LineNo:                   int32(row.LineNo),
			Mpn:                      row.Mpn,
			UnifiedMpn:               derefStrPtr(row.UnifiedMpn),
			ReferenceDesignator:      derefStrPtr(row.ReferenceDesignator),
			SubstituteMpn:            derefStrPtr(row.SubstituteMpn),
			Remark:                   derefStrPtr(row.Remark),
			Description:              derefStrPtr(row.Description),
			RawText:                  derefStrPtr(row.RawText),
			Mfr:                      derefStrPtr(row.Mfr),
			Package:                  derefStrPtr(row.Package),
			Qty:                      derefFloat(row.Qty),
			AvailabilityStatus:       lineAvailability.Status,
			AvailabilityReasonCode:   lineAvailability.ReasonCode,
			AvailabilityReason:       lineAvailability.Reason,
			HasUsableQuote:           lineAvailability.HasUsableQuote,
			RawQuotePlatformCount:    int32(lineAvailability.RawQuotePlatformCount),
			UsableQuotePlatformCount: int32(lineAvailability.UsableQuotePlatformCount),
			ResolutionStatus:         lineAvailability.ResolutionStatus,
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

func demandManufacturerCleaningRequired(lines []data.BomSessionLine) []int {
	var out []int
	for _, line := range lines {
		if strings.TrimSpace(derefStrPtr(line.Mfr)) != "" && line.ManufacturerCanonicalID == nil {
			out = append(out, line.LineNo)
		}
	}
	return out
}

func formatLineNos(lineNos []int) string {
	parts := make([]string, 0, len(lineNos))
	for _, lineNo := range lineNos {
		parts = append(parts, strconv.Itoa(lineNo))
	}
	return strings.Join(parts, ",")
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
			row, err = s.reconcileFinishedDispatchSearchTask(ctx, sid, view.BizDate, row)
			if err != nil {
				return nil, err
			}
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
	canon, err := s.canonicalPtrForManufacturer(ctx, req.GetMfr())
	if err != nil {
		return nil, err
	}
	id, lineNo, rev, err := s.session.CreateSessionLine(
		ctx,
		req.GetSessionId(),
		req.GetMpn(),
		req.GetUnifiedMpn(),
		req.GetReferenceDesignator(),
		req.GetSubstituteMpn(),
		req.GetRemark(),
		req.GetDescription(),
		req.GetMfr(),
		req.GetPackage(),
		canon,
		qty,
		raw,
		extra,
	)
	if err != nil {
		return nil, err
	}
	view, err := s.session.GetSession(ctx, req.GetSessionId())
	if err != nil {
		return nil, err
	}
	var pairs []biz.MpnPlatformPair
	keys := []string{biz.NormalizeMPNForBOMSearch(req.GetMpn())}
	if sub := biz.NormalizeMPNForBOMSearch(req.GetSubstituteMpn()); sub != "" && sub != keys[0] {
		keys = append(keys, sub)
	}
	for _, key := range keys {
		for _, p := range view.PlatformIDs {
			pairs = append(pairs, biz.MpnPlatformPair{MpnNorm: key, PlatformID: biz.NormalizePlatformID(p)})
		}
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
	var mpn, unifiedMpn, referenceDesignator, substituteMpn, remark, description, mfr, pkg, raw, extra *string
	var qty *float64
	if req.Mpn != nil {
		mpn = req.Mpn
	}
	if req.Mfr != nil {
		mfr = req.Mfr
	}
	if req.UnifiedMpn != nil {
		unifiedMpn = req.UnifiedMpn
	}
	if req.ReferenceDesignator != nil {
		referenceDesignator = req.ReferenceDesignator
	}
	if req.SubstituteMpn != nil {
		substituteMpn = req.SubstituteMpn
	}
	if req.Remark != nil {
		remark = req.Remark
	}
	if req.Description != nil {
		description = req.Description
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
	var mfrCanon biz.OptionalStringPtr
	if req.Mfr != nil {
		canon, err := s.canonicalPtrForManufacturer(ctx, *req.Mfr)
		if err != nil {
			return nil, err
		}
		mfrCanon = biz.OptionalStringPtr{Set: true, Value: canon}
	}
	rev, err := s.session.UpdateSessionLine(ctx, sid, lid, mpn, unifiedMpn, referenceDesignator, substituteMpn, remark, description, mfr, pkg, mfrCanon, qty, raw, extra)
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
	oldSub := ""
	newSub := ""
	for _, ln := range linesBefore {
		if ln.ID == lid {
			oldSub = derefStrPtr(ln.SubstituteMpn)
			break
		}
	}
	if substituteMpn != nil {
		newSub = *substituteMpn
	} else {
		newSub = oldSub
	}
	if biz.NormalizeMPNForBOMSearch(oldMpn) != biz.NormalizeMPNForBOMSearch(newMpn) {
		_ = s.search.CancelTasksBySessionMpnNorm(ctx, sid, oldMpn)
		var pairs []biz.MpnPlatformPair
		keys := []string{biz.NormalizeMPNForBOMSearch(newMpn)}
		if sub := biz.NormalizeMPNForBOMSearch(newSub); sub != "" && sub != keys[0] {
			keys = append(keys, sub)
		}
		for _, key := range keys {
			for _, p := range view.PlatformIDs {
				pairs = append(pairs, biz.MpnPlatformPair{MpnNorm: key, PlatformID: biz.NormalizePlatformID(p)})
			}
		}
		if err := s.search.UpsertPendingTasks(ctx, sid, view.BizDate, rev, pairs); err != nil {
			return nil, err
		}
		s.tryMergeDispatchSession(ctx, sid)
	} else if biz.NormalizeMPNForBOMSearch(oldSub) != biz.NormalizeMPNForBOMSearch(newSub) {
		if oldSub != "" {
			_ = s.search.CancelTasksBySessionMpnNorm(ctx, sid, oldSub)
		}
		var pairs []biz.MpnPlatformPair
		keys := []string{biz.NormalizeMPNForBOMSearch(newMpn)}
		if sub := biz.NormalizeMPNForBOMSearch(newSub); sub != "" && sub != keys[0] {
			keys = append(keys, sub)
		}
		for _, key := range keys {
			for _, p := range view.PlatformIDs {
				pairs = append(pairs, biz.MpnPlatformPair{MpnNorm: key, PlatformID: biz.NormalizePlatformID(p)})
			}
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

func formatOptTimeRFC3339(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}

func buildMpnPlatformPairs(lines []biz.BomImportLine, platforms []string) []biz.MpnPlatformPair {
	var pairs []biz.MpnPlatformPair
	for _, ln := range lines {
		keys := []string{biz.NormalizeMPNForBOMSearch(ln.Mpn)}
		if sub := biz.NormalizeMPNForBOMSearch(ln.SubstituteMpn); sub != "" && sub != keys[0] {
			keys = append(keys, sub)
		}
		for _, key := range keys {
			for _, p := range platforms {
				pairs = append(pairs, biz.MpnPlatformPair{MpnNorm: key, PlatformID: biz.NormalizePlatformID(p)})
			}
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
	_ = f.SetSheetRow(sheet, "A1", &[]any{"序号", "客户原型号", "统一型号", "品牌", "用量", "描述/规格", "封装", "位号", "替代型号", "备注"})
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
	if strings.TrimSpace(req.GetRunId()) != "" {
		runID, err := strconv.ParseUint(req.GetRunId(), 10, 64)
		if err != nil {
			return nil, kerrors.BadRequest("BAD_RUN_ID", "invalid run_id")
		}
		_, items, err := s.matchRuns.GetMatchRun(ctx, runID)
		if err != nil {
			return nil, err
		}
		return s.exportMatchRunItems(items, req.GetFormat())
	}
	rows, err := s.dataListLines(ctx, req.GetSessionId())
	if err != nil {
		return nil, err
	}
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	_ = f.SetSheetRow(sheet, "A1", &[]any{"序号", "客户原型号", "统一型号", "品牌", "用量", "描述/规格", "封装", "位号", "替代型号", "备注"})
	for i, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		qty := 0.0
		if row.Qty != nil {
			qty = *row.Qty
		}
		_ = f.SetSheetRow(sheet, cell, &[]any{
			row.LineNo,
			row.Mpn,
			derefStrPtr(row.UnifiedMpn),
			derefStrPtr(row.Mfr),
			qty,
			derefStrPtr(row.Description),
			derefStrPtr(row.Package),
			derefStrPtr(row.ReferenceDesignator),
			derefStrPtr(row.SubstituteMpn),
			derefStrPtr(row.Remark),
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
