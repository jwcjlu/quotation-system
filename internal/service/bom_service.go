package service

import (
	"context"
	stderrors "errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	pb "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/conf"
	"caichip/internal/data"
	"caichip/pkg/parser"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

// BomService BOM 服务实现
type BomService struct {
	pb.UnimplementedBomServiceServer

	bomUC              *biz.BOMUseCase
	sessionUC          *biz.BOMSessionUseCase
	matchUC            *biz.MatchUseCase
	searchRepo         biz.SearchRepo
	bomSearch          *data.BOMSearchTaskRepo
	searchCallbackKeys []string
	log                *log.Helper
}

// NewBomService 创建 BOM 服务
func NewBomService(
	bomUC *biz.BOMUseCase,
	sessionUC *biz.BOMSessionUseCase,
	matchUC *biz.MatchUseCase,
	searchRepo biz.SearchRepo,
	bomSearch *data.BOMSearchTaskRepo,
	bc *conf.Bootstrap,
	logger log.Logger,
) *BomService {
	var keys []string
	if bc != nil && bc.BomSearchCallback != nil {
		for _, k := range bc.BomSearchCallback.ApiKeys {
			k = strings.TrimSpace(k)
			if k != "" {
				keys = append(keys, k)
			}
		}
	}
	var helper *log.Helper
	if logger != nil {
		helper = log.NewHelper(log.With(logger, "service", "bom"))
	}
	return &BomService{
		bomUC:              bomUC,
		sessionUC:          sessionUC,
		matchUC:            matchUC,
		searchRepo:         searchRepo,
		bomSearch:          bomSearch,
		searchCallbackKeys: keys,
		log:                helper,
	}
}

func (s *BomService) warnf(format string, args ...interface{}) {
	if s == nil || s.log == nil {
		return
	}
	s.log.Warnf(format, args...)
}

func (s *BomService) errorf(format string, args ...interface{}) {
	if s == nil || s.log == nil {
		return
	}
	s.log.Errorf(format, args...)
}

// UploadBOM 上传并解析 BOM
func (s *BomService) UploadBOM(ctx context.Context, req *pb.UploadBOMRequest) (*pb.UploadBOMReply, error) {
	if len(req.File) == 0 {
		return nil, errors.BadRequest("FILE_EMPTY", "file is required")
	}

	mode := parser.ParseMode(strings.ToLower(strings.TrimSpace(req.ParseMode)))
	if mode == "" {
		mode = parser.ParseModeAuto
	}
	if mode != parser.ParseModeSZLCSC && mode != parser.ParseModeIckey &&
		mode != parser.ParseModeAuto && mode != parser.ParseModeCustom {
		mode = parser.ParseModeAuto
	}

	var mapping parser.ColumnMapping
	if req.ColumnMapping != nil {
		mapping = req.ColumnMapping
	}

	sessionID := strings.TrimSpace(req.GetSessionId())
	bom, err := s.bomUC.ParseAndSave(ctx, req.File, mode, mapping, sessionID)
	if err != nil {
		if err == biz.ErrBOMSessionNotFound {
			s.warnf("UploadBOM: session not found session_id=%q: %v", sessionID, err)
			return nil, errors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		if err == biz.ErrBOMSessionStoreUnavailable {
			s.warnf("UploadBOM: session store unavailable session_id=%q: %v", sessionID, err)
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		s.errorf("UploadBOM: parse/save failed session_id=%q: %v", sessionID, err)
		return nil, errors.InternalServer("PARSE_FAILED", err.Error())
	}

	items := make([]*pb.ParsedItem, len(bom.Items))
	for i, it := range bom.Items {
		items[i] = &pb.ParsedItem{
			Index:        int32(it.Index),
			Raw:          it.Raw,
			Model:        it.Model,
			Manufacturer: it.Manufacturer,
			Package:      it.Package,
			Quantity:     int32(it.Quantity),
			Params:       it.Params,
		}
	}

	return &pb.UploadBOMReply{
		BomId: bom.ID,
		Items: items,
		Total: int32(len(items)),
	}, nil
}

// SearchQuotes 经典多平台实时搜价（已停用，保留 RPC 以便客户端收到明确错误）
func (s *BomService) SearchQuotes(ctx context.Context, req *pb.SearchQuotesRequest) (*pb.SearchQuotesReply, error) {
	_ = ctx
	_ = req
	return nil, errors.New(501, "CLASSIC_SEARCH_DEPRECATED", "经典 BOM 多平台搜价已停用，请使用货源会话流程获取报价")
}

// AutoMatch 自动配单
func (s *BomService) AutoMatch(ctx context.Context, req *pb.AutoMatchRequest) (*pb.AutoMatchReply, error) {
	if req.BomId == "" {
		return nil, errors.BadRequest("BOM_ID_EMPTY", "bom_id is required")
	}

	strategy := strings.TrimSpace(req.Strategy)
	if strategy == "" {
		strategy = biz.StrategyPriceFirst
	}

	if _, err := uuid.Parse(strings.TrimSpace(req.BomId)); err == nil && s.bomSearch != nil && s.bomSearch.DBOk() {
		bomObj, gErr := s.bomUC.GetBOM(ctx, req.BomId)
		if gErr != nil {
			s.warnf("AutoMatch: skip DB quote refresh — GetBOM bom_id=%q: %v", req.BomId, gErr)
		} else if bomObj != nil {
			sess, sErr := s.sessionUC.GetSession(ctx, req.BomId)
			if sErr != nil {
				s.warnf("AutoMatch: skip DB quote refresh — GetSession bom_id=%q: %v", req.BomId, sErr)
			} else if sess != nil {
				quotes, lErr := biz.LoadItemQuotesForSession(ctx, s.bomSearch, bomObj, sess.BizDate)
				if lErr != nil {
					s.errorf("AutoMatch: LoadItemQuotesForSession bom_id=%q: %v", req.BomId, lErr)
				} else if len(quotes) > 0 {
					if saveErr := s.searchRepo.SaveQuotes(ctx, req.BomId, quotes); saveErr != nil {
						s.errorf("AutoMatch: SaveQuotes bom_id=%q: %v", req.BomId, saveErr)
					}
				}
			}
		}
	}

	items, totalAmount, err := s.matchUC.AutoMatch(ctx, req.BomId, strategy)
	if err != nil {
		if err == biz.ErrBOMNotFound {
			s.warnf("AutoMatch: BOM not found bom_id=%q", req.BomId)
			return nil, errors.NotFound("BOM_NOT_FOUND", "bom not found")
		}
		s.errorf("AutoMatch: match failed bom_id=%q strategy=%q: %v", req.BomId, strategy, err)
		return nil, errors.InternalServer("MATCH_FAILED", err.Error())
	}

	matchItems := make([]*pb.MatchItem, len(items))
	for i, m := range items {
		allQuotes := make([]*pb.PlatformQuote, len(m.AllQuotes))
		for j, q := range m.AllQuotes {
			allQuotes[j] = bizQuoteToPB(q)
		}
		matchItems[i] = &pb.MatchItem{
			Index:              int32(m.Index),
			Model:              m.Model,
			Quantity:           int32(m.Quantity),
			MatchedModel:       m.MatchedModel,
			Manufacturer:       m.Manufacturer,
			Platform:           m.Platform,
			LeadTime:           m.LeadTime,
			Stock:              m.Stock,
			UnitPrice:          m.UnitPrice,
			Subtotal:           m.Subtotal,
			MatchStatus:        m.MatchStatus,
			AllQuotes:          allQuotes,
			DemandManufacturer: m.DemandManufacturer,
			DemandPackage:      m.DemandPackage,
		}
	}

	return &pb.AutoMatchReply{
		Items:       matchItems,
		TotalAmount: totalAmount,
	}, nil
}

// GetBOM 获取 BOM 详情
func (s *BomService) GetBOM(ctx context.Context, req *pb.GetBOMRequest) (*pb.GetBOMReply, error) {
	if req.BomId == "" {
		return nil, errors.BadRequest("BOM_ID_EMPTY", "bom_id is required")
	}

	bom, err := s.bomUC.GetBOM(ctx, req.BomId)
	if err != nil {
		s.errorf("GetBOM: bom_id=%q: %v", req.BomId, err)
		return nil, errors.InternalServer("GET_FAILED", err.Error())
	}
	if bom == nil {
		s.warnf("GetBOM: bom not found bom_id=%q", req.BomId)
		return nil, errors.NotFound("BOM_NOT_FOUND", "bom not found")
	}

	items := make([]*pb.ParsedItem, len(bom.Items))
	for i, it := range bom.Items {
		items[i] = &pb.ParsedItem{
			Index:        int32(it.Index),
			Raw:          it.Raw,
			Model:        it.Model,
			Manufacturer: it.Manufacturer,
			Package:      it.Package,
			Quantity:     int32(it.Quantity),
			Params:       it.Params,
		}
	}

	return &pb.GetBOMReply{
		BomId:     bom.ID,
		CreatedAt: bom.CreatedAt.Format(time.RFC3339),
		Items:     items,
	}, nil
}

// GetMatchResult 获取配单结果
func (s *BomService) GetMatchResult(ctx context.Context, req *pb.GetMatchResultRequest) (*pb.GetMatchResultReply, error) {
	if req.BomId == "" {
		return nil, errors.BadRequest("BOM_ID_EMPTY", "bom_id is required")
	}

	items, err := s.searchRepo.GetMatchResult(ctx, req.BomId)
	if err != nil {
		s.errorf("GetMatchResult: GetMatchResult bom_id=%q: %v", req.BomId, err)
		return nil, errors.InternalServer("GET_FAILED", err.Error())
	}
	if items == nil {
		items, _, err = s.matchUC.AutoMatch(ctx, req.BomId, biz.StrategyPriceFirst)
		if err != nil {
			s.warnf("GetMatchResult: no cached match, AutoMatch failed bom_id=%q: %v", req.BomId, err)
			return nil, errors.NotFound("MATCH_NOT_FOUND", "match result not found, run AutoMatch first")
		}
	}

	var totalAmount float64
	matchItems := make([]*pb.MatchItem, len(items))
	for i, m := range items {
		totalAmount += m.Subtotal
		allQuotes := make([]*pb.PlatformQuote, len(m.AllQuotes))
		for j, q := range m.AllQuotes {
			allQuotes[j] = bizQuoteToPB(q)
		}
		matchItems[i] = &pb.MatchItem{
			Index:              int32(m.Index),
			Model:              m.Model,
			Quantity:           int32(m.Quantity),
			MatchedModel:       m.MatchedModel,
			Manufacturer:       m.Manufacturer,
			Platform:           m.Platform,
			LeadTime:           m.LeadTime,
			Stock:              m.Stock,
			UnitPrice:          m.UnitPrice,
			Subtotal:           m.Subtotal,
			MatchStatus:        m.MatchStatus,
			AllQuotes:          allQuotes,
			DemandManufacturer: m.DemandManufacturer,
			DemandPackage:      m.DemandPackage,
		}
	}

	return &pb.GetMatchResultReply{
		Items:       matchItems,
		TotalAmount: totalAmount,
	}, nil
}

// DownloadTemplate 下载 BOM 模板
func (s *BomService) DownloadTemplate(ctx context.Context, req *pb.DownloadTemplateRequest) (*pb.DownloadTemplateReply, error) {
	file, err := generateBOMTemplate()
	if err != nil {
		s.errorf("DownloadTemplate: generate failed: %v", err)
		return nil, errors.InternalServer("TEMPLATE_FAILED", fmt.Sprintf("generate template: %v", err))
	}
	return &pb.DownloadTemplateReply{
		File:     file,
		Filename: "bom_template.xlsx",
	}, nil
}

// CreateSession 创建 BOM 会话。
func (s *BomService) CreateSession(ctx context.Context, req *pb.CreateSessionRequest) (*pb.CreateSessionReply, error) {
	var in biz.SessionCreateInput
	if req != nil {
		in.Title = req.Title
		in.PlatformIDs = req.PlatformIds
		in.CustomerName = req.CustomerName
		in.ContactPhone = req.ContactPhone
		in.ContactEmail = req.ContactEmail
		in.ContactExtra = req.ContactExtra
	}
	sess, err := s.sessionUC.CreateSession(ctx, in)
	if err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			s.warnf("CreateSession: DB unavailable: %v", err)
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		s.errorf("CreateSession: failed: %v", err)
		return nil, errors.InternalServer("CREATE_SESSION_FAILED", err.Error())
	}
	return &pb.CreateSessionReply{
		SessionId:         sess.ID,
		BizDate:           sess.BizDate.Format("2006-01-02"),
		SelectionRevision: int32(sess.SelectionRevision),
	}, nil
}

// GetSession 获取会话详情。
func (s *BomService) GetSession(ctx context.Context, req *pb.GetSessionRequest) (*pb.GetSessionReply, error) {
	if req == nil || strings.TrimSpace(req.SessionId) == "" {
		return nil, errors.BadRequest("SESSION_ID_EMPTY", "session_id is required")
	}
	sess, err := s.sessionUC.GetSession(ctx, req.SessionId)
	if err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			s.warnf("GetSession: DB unavailable session_id=%q: %v", req.SessionId, err)
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		if err == biz.ErrBOMSessionNotFound {
			s.warnf("GetSession: not found session_id=%q", req.SessionId)
			return nil, errors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		s.errorf("GetSession: session_id=%q: %v", req.SessionId, err)
		return nil, errors.InternalServer("GET_SESSION_FAILED", err.Error())
	}
	return bomSessionToPB(sess), nil
}

// ListSessions 分页列出 BOM 会话。
func (s *BomService) ListSessions(ctx context.Context, req *pb.ListSessionsRequest) (*pb.ListSessionsReply, error) {
	var page, pageSize int
	if req != nil {
		page = int(req.Page)
		pageSize = int(req.PageSize)
	}
	filter := biz.SessionListFilter{
		Page:     page,
		PageSize: pageSize,
	}
	if req != nil {
		filter.Status = req.Status
		filter.BizDate = req.BizDate
		filter.Q = req.Q
	}
	rows, total, err := s.sessionUC.ListSessions(ctx, filter)
	if err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			s.warnf("ListSessions: DB unavailable: %v", err)
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		s.errorf("ListSessions: %v", err)
		return nil, errors.InternalServer("LIST_SESSIONS_FAILED", err.Error())
	}
	items := make([]*pb.SessionListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, &pb.SessionListItem{
			SessionId:    r.SessionID,
			Title:        r.Title,
			CustomerName: r.CustomerName,
			Status:       r.Status,
			BizDate:      r.BizDate.Format("2006-01-02"),
			UpdatedAt:    r.UpdatedAt.UTC().Format(time.RFC3339Nano),
			LineCount:    int32(r.LineCount),
		})
	}
	return &pb.ListSessionsReply{Items: items, Total: int32(total)}, nil
}

// PatchSession 更新会话头（标题、客户信息等）。
func (s *BomService) PatchSession(ctx context.Context, req *pb.PatchSessionRequest) (*pb.GetSessionReply, error) {
	if req == nil || strings.TrimSpace(req.SessionId) == "" {
		return nil, errors.BadRequest("SESSION_ID_EMPTY", "session_id is required")
	}
	patch := &biz.SessionHeaderPatch{}
	if req.Title != nil {
		patch.Title = req.Title
	}
	if req.CustomerName != nil {
		patch.CustomerName = req.CustomerName
	}
	if req.ContactPhone != nil {
		patch.ContactPhone = req.ContactPhone
	}
	if req.ContactEmail != nil {
		patch.ContactEmail = req.ContactEmail
	}
	if req.ContactExtra != nil {
		patch.ContactExtra = req.ContactExtra
	}
	if patch.Title == nil && patch.CustomerName == nil && patch.ContactPhone == nil && patch.ContactEmail == nil && patch.ContactExtra == nil {
		return nil, errors.BadRequest("PATCH_EMPTY", "no fields to update")
	}
	err := s.sessionUC.PatchSession(ctx, strings.TrimSpace(req.SessionId), patch)
	if err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		if err == biz.ErrBOMSessionNotFound {
			return nil, errors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		s.errorf("PatchSession: %v", err)
		return nil, errors.InternalServer("PATCH_SESSION_FAILED", err.Error())
	}
	sess, err := s.sessionUC.GetSession(ctx, strings.TrimSpace(req.SessionId))
	if err != nil {
		s.errorf("PatchSession: GetSession after patch: %v", err)
		return nil, errors.InternalServer("GET_SESSION_FAILED", err.Error())
	}
	return bomSessionToPB(sess), nil
}

// CreateSessionLine 在会话末尾追加一行。
func (s *BomService) CreateSessionLine(ctx context.Context, req *pb.CreateSessionLineRequest) (*pb.CreateSessionLineReply, error) {
	if req == nil || strings.TrimSpace(req.SessionId) == "" {
		return nil, errors.BadRequest("SESSION_ID_EMPTY", "session_id is required")
	}
	if strings.TrimSpace(req.Mpn) == "" {
		return nil, errors.BadRequest("MPN_EMPTY", "mpn is required")
	}
	line := &biz.BOMSessionLine{
		RawText: strings.TrimSpace(req.Raw),
		MPN:     strings.TrimSpace(req.Mpn),
		MFR:     strings.TrimSpace(req.Mfr),
		Package: strings.TrimSpace(req.Package),
	}
	if req.Qty != 0 {
		q := req.Qty
		line.Qty = &q
	}
	if strings.TrimSpace(req.ExtraJson) != "" {
		line.ExtraJSON = []byte(req.ExtraJson)
	}
	sid := strings.TrimSpace(req.SessionId)
	err := s.sessionUC.InsertSessionLine(ctx, sid, line)
	if err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		if err == biz.ErrBOMSessionNotFound {
			return nil, errors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		if err == biz.ErrBOMSessionLineMPNRequired {
			return nil, errors.BadRequest("MPN_EMPTY", "mpn is required")
		}
		s.errorf("CreateSessionLine: %v", err)
		return nil, errors.InternalServer("CREATE_LINE_FAILED", err.Error())
	}
	s.ensureBOMSearchTasks(ctx, sid)
	return &pb.CreateSessionLineReply{
		LineId: fmt.Sprintf("%d", line.ID),
		LineNo: int32(line.LineNo),
	}, nil
}

// PatchSessionLine 更新会话中的一行。
func (s *BomService) PatchSessionLine(ctx context.Context, req *pb.PatchSessionLineRequest) (*pb.PatchSessionLineReply, error) {
	if req == nil || strings.TrimSpace(req.SessionId) == "" {
		return nil, errors.BadRequest("SESSION_ID_EMPTY", "session_id is required")
	}
	lineID, err := strconv.ParseInt(strings.TrimSpace(req.LineId), 10, 64)
	if err != nil || lineID <= 0 {
		return nil, errors.BadRequest("LINE_ID_INVALID", "line_id must be a positive integer")
	}
	patch := &biz.BOMSessionLinePatch{}
	if req.Mpn != nil {
		patch.MPN = req.Mpn
	}
	if req.Mfr != nil {
		patch.MFR = req.Mfr
	}
	if req.Package != nil {
		patch.Package = req.Package
	}
	if req.Qty != nil {
		patch.Qty = req.Qty
	}
	if req.Raw != nil {
		patch.RawText = req.Raw
	}
	if req.ExtraJson != nil {
		b := []byte(*req.ExtraJson)
		patch.ExtraJSON = &b
	}
	if patch.MPN == nil && patch.MFR == nil && patch.Package == nil && patch.Qty == nil && patch.RawText == nil && patch.ExtraJSON == nil {
		return nil, errors.BadRequest("PATCH_EMPTY", "no fields to update")
	}
	sid := strings.TrimSpace(req.SessionId)
	err = s.sessionUC.UpdateSessionLine(ctx, sid, lineID, patch)
	if err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		if err == biz.ErrBOMSessionLineNotFound {
			return nil, errors.NotFound("LINE_NOT_FOUND", "line not found")
		}
		s.errorf("PatchSessionLine: %v", err)
		return nil, errors.InternalServer("PATCH_LINE_FAILED", err.Error())
	}
	s.ensureBOMSearchTasks(ctx, sid)
	lines, err := s.sessionUC.ListSessionLines(ctx, sid)
	if err != nil {
		return &pb.PatchSessionLineReply{LineId: req.LineId}, nil
	}
	for _, ln := range lines {
		if ln.ID == lineID {
			return &pb.PatchSessionLineReply{
				LineId: fmt.Sprintf("%d", ln.ID),
				LineNo: int32(ln.LineNo),
			}, nil
		}
	}
	return &pb.PatchSessionLineReply{LineId: req.LineId}, nil
}

// DeleteSessionLine 删除一行并重排 line_no。
func (s *BomService) DeleteSessionLine(ctx context.Context, req *pb.DeleteSessionLineRequest) (*pb.DeleteSessionLineReply, error) {
	if req == nil || strings.TrimSpace(req.SessionId) == "" {
		return nil, errors.BadRequest("SESSION_ID_EMPTY", "session_id is required")
	}
	lineID, err := strconv.ParseInt(strings.TrimSpace(req.LineId), 10, 64)
	if err != nil || lineID <= 0 {
		return nil, errors.BadRequest("LINE_ID_INVALID", "line_id must be a positive integer")
	}
	sid := strings.TrimSpace(req.SessionId)
	err = s.sessionUC.DeleteSessionLine(ctx, sid, lineID)
	if err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		if err == biz.ErrBOMSessionLineNotFound {
			return nil, errors.NotFound("LINE_NOT_FOUND", "line not found")
		}
		s.errorf("DeleteSessionLine: %v", err)
		return nil, errors.InternalServer("DELETE_LINE_FAILED", err.Error())
	}
	s.ensureBOMSearchTasks(ctx, sid)
	return &pb.DeleteSessionLineReply{}, nil
}

func (s *BomService) ensureBOMSearchTasks(ctx context.Context, sessionID string) {
	if s.bomSearch != nil && s.bomSearch.DBOk() {
		if e := s.bomSearch.EnsureTasksForSession(ctx, sessionID); e != nil {
			s.warnf("EnsureTasksForSession session_id=%q: %v", sessionID, e)
		}
	}
}

func bomSessionToPB(sess *biz.BOMSession) *pb.GetSessionReply {
	if sess == nil {
		return &pb.GetSessionReply{}
	}
	return &pb.GetSessionReply{
		SessionId:         sess.ID,
		Title:             sess.Title,
		Status:            sess.Status,
		BizDate:           sess.BizDate.Format("2006-01-02"),
		SelectionRevision: int32(sess.SelectionRevision),
		PlatformIds:       sess.PlatformIDs,
		CustomerName:      sess.CustomerName,
		ContactPhone:      sess.ContactPhone,
		ContactEmail:      sess.ContactEmail,
		ContactExtra:      sess.ContactExtra,
	}
}

// PutPlatforms 更新勾选平台并递增 revision。
func (s *BomService) PutPlatforms(ctx context.Context, req *pb.PutPlatformsRequest) (*pb.PutPlatformsReply, error) {
	if req == nil || strings.TrimSpace(req.SessionId) == "" {
		return nil, errors.BadRequest("SESSION_ID_EMPTY", "session_id is required")
	}
	if len(req.PlatformIds) == 0 {
		return nil, errors.BadRequest("PLATFORM_IDS_EMPTY", "at least one platform_id required")
	}
	newRev, err := s.sessionUC.PutPlatforms(ctx, strings.TrimSpace(req.SessionId), req.PlatformIds, int(req.GetExpectedRevision()))
	if err != nil {
		sidLog := strings.TrimSpace(req.SessionId)
		if err == biz.ErrBOMSessionStoreUnavailable {
			s.warnf("PutPlatforms: DB unavailable session_id=%q: %v", sidLog, err)
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		if err == biz.ErrBOMSessionNotFound {
			s.warnf("PutPlatforms: session not found session_id=%q", sidLog)
			return nil, errors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		if err == biz.ErrBOMSessionRevisionConflict {
			s.warnf("PutPlatforms: revision conflict session_id=%q", sidLog)
			return nil, errors.Conflict("REVISION_MISMATCH", "selection_revision changed; refresh session and retry")
		}
		if err == biz.ErrBOMSessionPlatformsEmpty {
			s.warnf("PutPlatforms: empty platforms session_id=%q", sidLog)
			return nil, errors.BadRequest("PLATFORM_IDS_EMPTY", "at least one platform_id required")
		}
		s.errorf("PutPlatforms: session_id=%q: %v", sidLog, err)
		return nil, errors.InternalServer("PUT_PLATFORMS_FAILED", err.Error())
	}
	sid := strings.TrimSpace(req.SessionId)
	if s.bomSearch != nil && s.bomSearch.DBOk() {
		if e2 := s.bomSearch.EnsureTasksForSession(ctx, sid); e2 != nil {
			s.warnf("PutPlatforms: EnsureTasksForSession session_id=%q: %v", sid, e2)
		}
	}
	return &pb.PutPlatformsReply{SelectionRevision: int32(newRev)}, nil
}

// GetReadiness 会话级就绪轮询。
func (s *BomService) GetReadiness(ctx context.Context, req *pb.GetReadinessRequest) (*pb.GetReadinessReply, error) {
	if req == nil || strings.TrimSpace(req.SessionId) == "" {
		return nil, errors.BadRequest("SESSION_ID_EMPTY", "session_id is required")
	}
	sid := strings.TrimSpace(req.SessionId)
	sess, err := s.sessionUC.GetSession(ctx, sid)
	if err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			s.warnf("GetReadiness: DB unavailable session_id=%q: %v", sid, err)
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		if err == biz.ErrBOMSessionNotFound {
			s.warnf("GetReadiness: session not found session_id=%q", sid)
			return nil, errors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		s.errorf("GetReadiness: GetSession session_id=%q: %v", sid, err)
		return nil, errors.InternalServer("GET_READINESS_FAILED", err.Error())
	}

	nLines, err := s.sessionUC.CountSessionLines(ctx, sid)
	if err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			s.warnf("GetReadiness: DB unavailable (lines) session_id=%q: %v", sid, err)
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		s.errorf("GetReadiness: CountSessionLines session_id=%q: %v", sid, err)
		return nil, errors.InternalServer("GET_READINESS_FAILED", err.Error())
	}

	reply := &pb.GetReadinessReply{
		SessionId:         sess.ID,
		BizDate:           sess.BizDate.Format("2006-01-02"),
		SelectionRevision: int32(sess.SelectionRevision),
		Phase:             "idle",
		CanEnterMatch:     false,
		BlockReason:       "",
	}

	if nLines == 0 {
		reply.Phase = "awaiting_bom"
		reply.BlockReason = "请上传 BOM"
		return reply, nil
	}
	if len(sess.PlatformIDs) == 0 {
		reply.Phase = "awaiting_platforms"
		reply.BlockReason = "请勾选并保存货源平台"
		return reply, nil
	}

	if s.bomSearch != nil && s.bomSearch.DBOk() {
		if err := s.bomSearch.EnsureTasksForSession(ctx, sid); err != nil {
			if err == biz.ErrBOMSessionNotFound {
				s.warnf("GetReadiness: session not found while EnsureTasks session_id=%q", sid)
				return nil, errors.NotFound("SESSION_NOT_FOUND", "session not found")
			}
			s.errorf("GetReadiness: EnsureTasksForSession session_id=%q: %v", sid, err)
			return nil, errors.InternalServer("GET_READINESS_FAILED", err.Error())
		}
		agg, err := s.bomSearch.AggregateTasksForSession(ctx, sid, sess.BizDate)
		if err != nil {
			s.errorf("GetReadiness: AggregateTasksForSession session_id=%q: %v", sid, err)
			return nil, errors.InternalServer("GET_READINESS_FAILED", err.Error())
		}
		switch {
		case agg.Total == 0:
			reply.Phase = "queued"
			reply.BlockReason = "已排队生成搜索任务"
		case agg.PendingLike > 0:
			reply.Phase = "searching"
			reply.BlockReason = "搜索任务未完成"
		case agg.FailedLike > 0:
			reply.Phase = "blocked"
			reply.BlockReason = "存在失败或已取消的任务，请重试或检查 Agent"
		case agg.Succeeded == agg.Total:
			reply.Phase = "ready"
			reply.CanEnterMatch = true
		default:
			reply.Phase = "searching"
			reply.BlockReason = "任务状态异常，请稍后重试"
		}
		return reply, nil
	}

	reply.Phase = "degraded"
	reply.BlockReason = "数据库未配置搜索任务表或不可用"
	return reply, nil
}

// GetBOMLines 行列表与 platform_gaps。
func (s *BomService) GetBOMLines(ctx context.Context, req *pb.GetBOMLinesRequest) (*pb.GetBOMLinesReply, error) {
	if req == nil || strings.TrimSpace(req.SessionId) == "" {
		return nil, errors.BadRequest("SESSION_ID_EMPTY", "session_id is required")
	}
	sid := strings.TrimSpace(req.SessionId)
	sess, err := s.sessionUC.GetSession(ctx, sid)
	if err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			s.warnf("GetBOMLines: DB unavailable session_id=%q: %v", sid, err)
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		if err == biz.ErrBOMSessionNotFound {
			s.warnf("GetBOMLines: session not found session_id=%q", sid)
			return nil, errors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		s.errorf("GetBOMLines: GetSession session_id=%q: %v", sid, err)
		return nil, errors.InternalServer("GET_LINES_FAILED", err.Error())
	}

	lines, err := s.sessionUC.ListSessionLines(ctx, sid)
	if err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			s.warnf("GetBOMLines: DB unavailable (lines) session_id=%q: %v", sid, err)
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		s.errorf("GetBOMLines: ListSessionLines session_id=%q: %v", sid, err)
		return nil, errors.InternalServer("GET_LINES_FAILED", err.Error())
	}

	var tasks []data.SearchTaskRow
	if s.bomSearch != nil && s.bomSearch.DBOk() {
		if err := s.bomSearch.EnsureTasksForSession(ctx, sid); err != nil {
			if err == biz.ErrBOMSessionNotFound {
				s.warnf("GetBOMLines: session not found while EnsureTasks session_id=%q", sid)
				return nil, errors.NotFound("SESSION_NOT_FOUND", "session not found")
			}
			s.errorf("GetBOMLines: EnsureTasksForSession session_id=%q: %v", sid, err)
			return nil, errors.InternalServer("GET_LINES_FAILED", err.Error())
		}
		tasks, err = s.bomSearch.ListTasksForSession(ctx, sid, sess.BizDate)
		if err != nil {
			s.errorf("GetBOMLines: ListTasksForSession session_id=%q: %v", sid, err)
			return nil, errors.InternalServer("GET_LINES_FAILED", err.Error())
		}
	}

	taskByKey := make(map[string]data.SearchTaskRow, len(tasks))
	for _, t := range tasks {
		k := normMPNKey(t.MpnNorm) + "\x00" + strings.TrimSpace(t.PlatformID)
		taskByKey[k] = t
	}

	out := make([]*pb.BOMLineRow, 0, len(lines))
	for _, ln := range lines {
		if ln == nil {
			continue
		}
		qty := 0.0
		if ln.Qty != nil {
			qty = *ln.Qty
		}
		row := &pb.BOMLineRow{
			LineId:       fmt.Sprintf("%d", ln.ID),
			LineNo:       int32(ln.LineNo),
			Mpn:          ln.MPN,
			Mfr:          ln.MFR,
			Package:      ln.Package,
			Qty:          qty,
			MatchStatus:  "ok",
			PlatformGaps: nil,
		}

		if len(sess.PlatformIDs) == 0 {
			row.MatchStatus = "no_platforms"
			out = append(out, row)
			continue
		}

		var gaps []*pb.PlatformGap
		mpnK := normMPNKey(ln.MPN)
		anyPending := false
		anyFail := false
		for _, pid := range sess.PlatformIDs {
			p := strings.TrimSpace(pid)
			if p == "" {
				continue
			}
			t, ok := taskByKey[mpnK+"\x00"+p]
			if !ok {
				gaps = append(gaps, &pb.PlatformGap{
					PlatformId:    p,
					Phase:         "missing_task",
					ReasonCode:    "not_scheduled",
					Message:       "任务未生成，请刷新或重新上传",
					AutoAttempt:   0,
					ManualAttempt: 0,
					SearchUiState: biz.SearchUIMissing,
				})
				anyPending = true
				continue
			}
			qo := ""
			if s.bomSearch != nil {
				qo, _ = s.bomSearch.QuoteCacheOutcome(ctx, mpnK, p, sess.BizDate)
			}
			if g := platformGapFromTask(t, qo); g != nil {
				if g.SearchUiState == "" {
					g.SearchUiState = biz.MapSearchTaskStateToQuad(t.State)
				}
				gaps = append(gaps, g)
				st := strings.ToLower(strings.TrimSpace(t.State))
				if st == "failed" || st == "cancelled" {
					anyFail = true
				}
				if st == "pending" || st == "dispatched" || st == "running" {
					anyPending = true
				}
			} else {
				gaps = append(gaps, &pb.PlatformGap{
					PlatformId:    p,
					Phase:         "ok",
					ReasonCode:    "",
					Message:       "",
					AutoAttempt:   int32(t.AutoAttempt),
					ManualAttempt: int32(t.ManualAttempt),
					SearchUiState: biz.SearchUISucceeded,
				})
			}
		}
		row.PlatformGaps = gaps
		switch {
		case len(gaps) == 0:
			row.MatchStatus = "ok"
		case anyFail:
			row.MatchStatus = "error"
		case anyPending:
			row.MatchStatus = "pending"
		default:
			row.MatchStatus = "gaps"
		}
		out = append(out, row)
	}

	return &pb.GetBOMLinesReply{Lines: out}, nil
}

// GetSessionSearchTaskCoverage 只读检查：行×平台 与 bom_search_task 是否齐全（不写入）。
func (s *BomService) GetSessionSearchTaskCoverage(ctx context.Context, req *pb.GetSessionSearchTaskCoverageRequest) (*pb.GetSessionSearchTaskCoverageReply, error) {
	if req == nil || strings.TrimSpace(req.SessionId) == "" {
		return nil, errors.BadRequest("SESSION_ID_EMPTY", "session_id is required")
	}
	sid := strings.TrimSpace(req.SessionId)
	if _, err := s.sessionUC.GetSession(ctx, sid); err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		if err == biz.ErrBOMSessionNotFound {
			return nil, errors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		return nil, errors.InternalServer("GET_COVERAGE_FAILED", err.Error())
	}
	if s.bomSearch == nil || !s.bomSearch.DBOk() {
		return &pb.GetSessionSearchTaskCoverageReply{
			Consistent:        true,
			OrphanTaskCount:   0,
			ExpectedTaskCount: 0,
			ExistingTaskCount: 0,
			MissingTasks:      nil,
		}, nil
	}
	rep, err := s.bomSearch.ComputeSearchTaskCoverage(ctx, sid)
	if err != nil {
		if err == biz.ErrBOMSessionNotFound {
			return nil, errors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		s.errorf("GetSessionSearchTaskCoverage: %v", err)
		return nil, errors.InternalServer("GET_COVERAGE_FAILED", err.Error())
	}
	out := &pb.GetSessionSearchTaskCoverageReply{
		Consistent:        rep.Consistent,
		OrphanTaskCount:   int32(rep.OrphanTaskCount),
		ExpectedTaskCount: int32(rep.ExpectedTaskCount),
		ExistingTaskCount: int32(rep.ExistingTaskCount),
	}
	for _, m := range rep.Missing {
		out.MissingTasks = append(out.MissingTasks, &pb.SearchTaskMissingItem{
			LineId:     m.LineID,
			LineNo:     m.LineNo,
			MpnNorm:    m.MpnNorm,
			PlatformId: m.PlatformID,
			Reason:     m.Reason,
		})
	}
	return out, nil
}

// RetrySearchTasks 手动重试（manual_attempt）。
func (s *BomService) RetrySearchTasks(ctx context.Context, req *pb.RetrySearchTasksRequest) (*pb.RetrySearchTasksReply, error) {
	if req == nil || strings.TrimSpace(req.SessionId) == "" {
		return nil, errors.BadRequest("SESSION_ID_EMPTY", "session_id is required")
	}
	if s.bomSearch == nil || !s.bomSearch.DBOk() {
		s.warnf("RetrySearchTasks: bom search DB unavailable")
		return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM search tasks")
	}
	sid := strings.TrimSpace(req.SessionId)
	if _, err := s.sessionUC.GetSession(ctx, sid); err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			s.warnf("RetrySearchTasks: session DB unavailable session_id=%q: %v", sid, err)
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		if err == biz.ErrBOMSessionNotFound {
			s.warnf("RetrySearchTasks: session not found session_id=%q", sid)
			return nil, errors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		s.errorf("RetrySearchTasks: GetSession session_id=%q: %v", sid, err)
		return nil, errors.InternalServer("RETRY_TASKS_FAILED", err.Error())
	}

	var accepted int32
	for _, it := range req.Items {
		if it == nil {
			continue
		}
		mpn := strings.TrimSpace(it.Mpn)
		pid := strings.TrimSpace(it.PlatformId)
		if mpn == "" || pid == "" {
			continue
		}
		if err := s.bomSearch.BumpManualRetry(ctx, sid, mpn, pid); err != nil {
			s.errorf("RetrySearchTasks: BumpManualRetry session_id=%q mpn=%q platform=%q: %v", sid, mpn, pid, err)
			return nil, errors.InternalServer("RETRY_TASKS_FAILED", err.Error())
		}
		accepted++
	}
	return &pb.RetrySearchTasksReply{Accepted: accepted}, nil
}

// SubmitBomSearchResult Agent 回写单行搜索任务（bom_search_task + bom_quote_cache）。
func (s *BomService) SubmitBomSearchResult(ctx context.Context, req *pb.SubmitBomSearchResultRequest) (*pb.SubmitBomSearchResultReply, error) {
	if err := s.authorizeSearchCallback(ctx); err != nil {
		return nil, err
	}
	if req == nil {
		s.warnf("SubmitBomSearchResult: request nil")
		return nil, errors.BadRequest("REQUEST_EMPTY", "request is required")
	}
	sid := strings.TrimSpace(req.SessionId)
	if sid == "" {
		s.warnf("SubmitBomSearchResult: empty session_id")
		return nil, errors.BadRequest("SESSION_ID_EMPTY", "session_id is required")
	}
	if s.bomSearch == nil || !s.bomSearch.DBOk() {
		s.warnf("SubmitBomSearchResult: bom search DB unavailable session_id=%q", sid)
		return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM search tasks")
	}
	sess, err := s.sessionUC.GetSession(ctx, sid)
	if err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			s.warnf("SubmitBomSearchResult: session DB unavailable session_id=%q: %v", sid, err)
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		if err == biz.ErrBOMSessionNotFound {
			s.warnf("SubmitBomSearchResult: session not found session_id=%q", sid)
			return nil, errors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		s.errorf("SubmitBomSearchResult: GetSession session_id=%q: %v", sid, err)
		return nil, errors.InternalServer("GET_SESSION_FAILED", err.Error())
	}

	pid := strings.TrimSpace(req.PlatformId)
	if pid == "" {
		s.warnf("SubmitBomSearchResult: empty platform_id session_id=%q", sid)
		return nil, errors.BadRequest("PLATFORM_ID_EMPTY", "platform_id is required")
	}
	st := strings.ToLower(strings.TrimSpace(req.Status))
	switch st {
	case "succeeded_quotes", "succeeded_no_mpn", "failed":
	default:
		s.warnf("SubmitBomSearchResult: invalid status %q session_id=%q platform_id=%q", req.Status, sid, pid)
		return nil, errors.BadRequest("INVALID_STATUS", "status must be succeeded_quotes, succeeded_no_mpn or failed")
	}

	var lastErr *string
	if msg := strings.TrimSpace(req.ErrorMessage); msg != "" && st == "failed" {
		lastErr = &msg
	}
	qj := strings.TrimSpace(req.QuotesJson)
	var qbytes []byte
	if qj != "" {
		qbytes = []byte(qj)
	}
	ndj := strings.TrimSpace(req.NoMpnDetailJson)
	var ndbytes []byte
	if ndj != "" {
		ndbytes = []byte(ndj)
	}

	rawMpn := strings.TrimSpace(req.MpnNorm)
	if rawMpn == "" {
		s.warnf("SubmitBomSearchResult: empty mpn_norm session_id=%q platform_id=%q", sid, pid)
		return nil, errors.BadRequest("MPN_EMPTY", "mpn_norm is required")
	}
	mpnNorm := biz.NormalizeMPNForTask(rawMpn)

	caichipID := strings.TrimSpace(req.CaichipTaskId)
	ferr := s.bomSearch.FinalizeSearchTask(ctx, sid, mpnNorm, pid, sess.BizDate, caichipID, st, lastErr, "ok", qbytes, ndbytes)
	if stderrors.Is(ferr, data.ErrSearchTaskNotFound) {
		s.warnf("SubmitBomSearchResult: search task not found session_id=%q mpn_norm=%q platform_id=%q", sid, mpnNorm, pid)
		return nil, errors.NotFound("SEARCH_TASK_NOT_FOUND", "no bom_search_task for this session/mpn/platform/biz_date")
	}
	if stderrors.Is(ferr, data.ErrSearchTaskCaichipMismatch) {
		s.warnf("SubmitBomSearchResult: caichip_task_id mismatch session_id=%q platform_id=%q mpn_norm=%q caichip_id=%q", sid, pid, mpnNorm, caichipID)
		return nil, errors.Conflict("SEARCH_TASK_ID_MISMATCH", "caichip_task_id does not match the scheduled task")
	}
	if ferr != nil {
		s.errorf("SubmitBomSearchResult: FinalizeSearchTask session_id=%q platform_id=%q mpn_norm=%q status=%q: %v", sid, pid, mpnNorm, st, ferr)
		return nil, errors.InternalServer("SUBMIT_SEARCH_RESULT_FAILED", ferr.Error())
	}
	return &pb.SubmitBomSearchResultReply{
		Accepted:   true,
		ServerTime: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *BomService) authorizeSearchCallback(ctx context.Context) error {
	_ = ctx
	if len(s.searchCallbackKeys) == 0 {
		s.warnf("SubmitBomSearchResult: SEARCH_CALLBACK_NOT_CONFIGURED (bom_search_callback.api_keys empty)")
		return errors.ServiceUnavailable("SEARCH_CALLBACK_NOT_CONFIGURED", "bom_search_callback.api_keys empty in config")
	}
	var auth, xkey string
	if r, ok := khttp.RequestFromServerContext(ctx); ok {
		auth = r.Header.Get("Authorization")
		xkey = r.Header.Get("X-API-Key")
	} else if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get("x-api-key"); len(v) > 0 {
			xkey = v[0]
		}
		if v := md.Get("authorization"); len(v) > 0 {
			auth = v[0]
		}
	}
	if searchCallbackValidateKeys(s.searchCallbackKeys, auth, xkey) {
		return nil
	}
	s.warnf("SubmitBomSearchResult: unauthorized (invalid or missing API key)")
	return errors.Unauthorized("UNAUTHORIZED", "invalid or missing bom search callback key")
}

func searchCallbackValidateKeys(keys []string, authBearer, xAPIKey string) bool {
	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if k != "" {
			set[k] = struct{}{}
		}
	}
	if len(set) == 0 {
		return false
	}
	if strings.TrimSpace(xAPIKey) != "" {
		_, ok := set[strings.TrimSpace(xAPIKey)]
		return ok
	}
	const p = "Bearer "
	if strings.HasPrefix(authBearer, p) {
		token := strings.TrimSpace(authBearer[len(p):])
		_, ok := set[token]
		return ok
	}
	return false
}

func normMPNKey(mpn string) string {
	m := strings.TrimSpace(mpn)
	if m == "" {
		return "-"
	}
	return strings.ToUpper(m)
}

func platformGapFromTask(t data.SearchTaskRow, quoteOutcome string) *pb.PlatformGap {
	st := strings.ToLower(strings.TrimSpace(t.State))
	ui := biz.MapSearchTaskStateToQuad(t.State)
	switch st {
	case "succeeded_quotes":
		if quoteOutcome != "" {
			lo := strings.ToLower(quoteOutcome)
			if strings.Contains(lo, "error") || strings.Contains(lo, "fail") {
				return &pb.PlatformGap{
					PlatformId:    t.PlatformID,
					Phase:         st,
					ReasonCode:    quoteOutcome,
					Message:       "报价缓存标记异常",
					AutoAttempt:   int32(t.AutoAttempt),
					ManualAttempt: int32(t.ManualAttempt),
					SearchUiState: ui,
				}
			}
		}
		return nil
	case "succeeded_no_mpn":
		return &pb.PlatformGap{
			PlatformId:    t.PlatformID,
			Phase:         st,
			ReasonCode:    "no_mpn",
			Message:       "未有匹配型号",
			AutoAttempt:   int32(t.AutoAttempt),
			ManualAttempt: int32(t.ManualAttempt),
			SearchUiState: ui,
		}
	case "pending", "dispatched", "running":
		msg := "等待 Agent 执行"
		if st == "running" {
			msg = "执行中"
		}
		return &pb.PlatformGap{
			PlatformId:    t.PlatformID,
			Phase:         st,
			ReasonCode:    "",
			Message:       msg,
			AutoAttempt:   int32(t.AutoAttempt),
			ManualAttempt: int32(t.ManualAttempt),
			SearchUiState: ui,
		}
	case "failed", "cancelled":
		msg := ""
		if t.LastError.Valid {
			msg = t.LastError.String
		}
		if msg == "" {
			msg = "任务失败"
		}
		return &pb.PlatformGap{
			PlatformId:    t.PlatformID,
			Phase:         st,
			ReasonCode:    st,
			Message:       msg,
			AutoAttempt:   int32(t.AutoAttempt),
			ManualAttempt: int32(t.ManualAttempt),
			SearchUiState: ui,
		}
	default:
		return &pb.PlatformGap{
			PlatformId:    t.PlatformID,
			Phase:         st,
			ReasonCode:    "unknown_state",
			Message:       st,
			AutoAttempt:   int32(t.AutoAttempt),
			ManualAttempt: int32(t.ManualAttempt),
			SearchUiState: ui,
		}
	}
}

// ExportSession 导出会话 BOM 行（Excel/CSV）。
func (s *BomService) ExportSession(ctx context.Context, req *pb.ExportSessionRequest) (*pb.ExportSessionReply, error) {
	if req == nil || strings.TrimSpace(req.SessionId) == "" {
		return nil, errors.BadRequest("SESSION_ID_EMPTY", "session_id is required")
	}
	sid := strings.TrimSpace(req.SessionId)
	if _, err := s.sessionUC.GetSession(ctx, sid); err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			s.warnf("ExportSession: DB unavailable session_id=%q: %v", sid, err)
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		if err == biz.ErrBOMSessionNotFound {
			s.warnf("ExportSession: session not found session_id=%q", sid)
			return nil, errors.NotFound("SESSION_NOT_FOUND", "session not found")
		}
		s.errorf("ExportSession: GetSession session_id=%q: %v", sid, err)
		return nil, errors.InternalServer("EXPORT_FAILED", err.Error())
	}
	lines, err := s.sessionUC.ListSessionLines(ctx, sid)
	if err != nil {
		if err == biz.ErrBOMSessionStoreUnavailable {
			s.warnf("ExportSession: DB unavailable (lines) session_id=%q: %v", sid, err)
			return nil, errors.ServiceUnavailable("DB_UNAVAILABLE", "database not configured for BOM sessions")
		}
		s.errorf("ExportSession: ListSessionLines session_id=%q: %v", sid, err)
		return nil, errors.InternalServer("EXPORT_FAILED", err.Error())
	}
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = "xlsx"
	}
	var file []byte
	var name string
	switch format {
	case "csv":
		file, name, err = exportSessionLinesToCSV(lines, sid)
	default:
		file, name, err = exportSessionLinesToXLSX(lines, sid)
	}
	if err != nil {
		s.errorf("ExportSession: build file session_id=%q format=%q: %v", sid, format, err)
		return nil, errors.InternalServer("EXPORT_FAILED", err.Error())
	}
	return &pb.ExportSessionReply{File: file, Filename: name}, nil
}

func bizMatchItemToPB(m *biz.MatchItem) *pb.MatchItem {
	allQuotes := make([]*pb.PlatformQuote, len(m.AllQuotes))
	for j, q := range m.AllQuotes {
		allQuotes[j] = bizQuoteToPB(q)
	}
	return &pb.MatchItem{
		Index:              int32(m.Index),
		Model:              m.Model,
		Quantity:           int32(m.Quantity),
		MatchedModel:       m.MatchedModel,
		Manufacturer:       m.Manufacturer,
		Platform:           m.Platform,
		LeadTime:           m.LeadTime,
		Stock:              m.Stock,
		UnitPrice:          m.UnitPrice,
		Subtotal:           m.Subtotal,
		MatchStatus:        m.MatchStatus,
		AllQuotes:          allQuotes,
		DemandManufacturer: m.DemandManufacturer,
		DemandPackage:      m.DemandPackage,
	}
}

func bizQuoteToPB(q *biz.Quote) *pb.PlatformQuote {
	return &pb.PlatformQuote{
		Platform:      q.Platform,
		MatchedModel:  q.MatchedModel,
		Manufacturer:  q.Manufacturer,
		Package:       q.Package,
		Description:   q.Description,
		Stock:         q.Stock,
		LeadTime:      q.LeadTime,
		Moq:           q.MOQ,
		Increment:     q.Increment,
		PriceTiers:    q.PriceTiers,
		HkPrice:       q.HKPrice,
		MainlandPrice: q.MainlandPrice,
		UnitPrice:     q.UnitPrice,
		Subtotal:      q.Subtotal,
	}
}
