package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	pb "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/pkg/parser"

	"github.com/go-kratos/kratos/v2/errors"
)

// BomService BOM 服务实现
type BomService struct {
	pb.UnimplementedBomServiceServer

	bomUC      *biz.BOMUseCase
	searchUC   *biz.SearchUseCase
	matchUC    *biz.MatchUseCase
	searchRepo biz.SearchRepo
}

// NewBomService 创建 BOM 服务
func NewBomService(
	bomUC *biz.BOMUseCase,
	searchUC *biz.SearchUseCase,
	matchUC *biz.MatchUseCase,
	searchRepo biz.SearchRepo,
) *BomService {
	return &BomService{
		bomUC:      bomUC,
		searchUC:   searchUC,
		matchUC:    matchUC,
		searchRepo: searchRepo,
	}
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

	bom, err := s.bomUC.ParseAndSave(ctx, req.File, mode, mapping)
	if err != nil {
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

// SearchQuotes 多平台搜索报价
func (s *BomService) SearchQuotes(ctx context.Context, req *pb.SearchQuotesRequest) (*pb.SearchQuotesReply, error) {
	if req.BomId == "" {
		return nil, errors.BadRequest("BOM_ID_EMPTY", "bom_id is required")
	}

	results, err := s.searchUC.SearchQuotes(ctx, req.BomId, req.Platforms)
	if err != nil {
		if err == biz.ErrBOMNotFound {
			return nil, errors.NotFound("BOM_NOT_FOUND", "bom not found")
		}
		return nil, errors.InternalServer("SEARCH_FAILED", err.Error())
	}

	_ = s.searchRepo.SaveQuotes(ctx, req.BomId, results)

	itemQuotes := make([]*pb.ItemQuotes, len(results))
	for i, iq := range results {
		quotes := make([]*pb.PlatformQuote, len(iq.Quotes))
		for j, q := range iq.Quotes {
			quotes[j] = bizQuoteToPB(q)
		}
		itemQuotes[i] = &pb.ItemQuotes{
			Model:    iq.Model,
			Quantity: int32(iq.Quantity),
			Quotes:   quotes,
		}
	}

	return &pb.SearchQuotesReply{ItemQuotes: itemQuotes}, nil
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

	items, totalAmount, err := s.matchUC.AutoMatch(ctx, req.BomId, strategy)
	if err != nil {
		if err == biz.ErrBOMNotFound {
			return nil, errors.NotFound("BOM_NOT_FOUND", "bom not found")
		}
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
		return nil, errors.InternalServer("GET_FAILED", err.Error())
	}
	if bom == nil {
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
		return nil, errors.InternalServer("GET_FAILED", err.Error())
	}
	if items == nil {
		items, _, err = s.matchUC.AutoMatch(ctx, req.BomId, biz.StrategyPriceFirst)
		if err != nil {
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
		return nil, errors.InternalServer("TEMPLATE_FAILED", fmt.Sprintf("generate template: %v", err))
	}
	return &pb.DownloadTemplateReply{
		File:     file,
		Filename: "bom_template.xlsx",
	}, nil
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
