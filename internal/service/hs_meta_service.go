package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"gorm.io/gorm"
)

// HsMetaService HS 元数据 HTTP 实现。
type HsMetaService struct {
	repo biz.HsMetaRepo
}

// NewHsMetaService ...
func NewHsMetaService(repo biz.HsMetaRepo) *HsMetaService {
	return &HsMetaService{repo: repo}
}

func parseHsMetaEnabledQuery(s string) *bool {
	t := strings.TrimSpace(strings.ToLower(s))
	if t == "" {
		return nil
	}
	if t == "1" || t == "true" {
		v := true
		return &v
	}
	if t == "0" || t == "false" {
		v := false
		return &v
	}
	return nil
}

func (s *HsMetaService) ListHsMeta(ctx context.Context, req *v1.HsMetaListRequest) (*v1.HsMetaListReply, error) {
	if s.repo == nil || !s.repo.DBOk() {
		return &v1.HsMetaListReply{Items: nil, Total: 0}, nil
	}
	filter := biz.HsMetaListFilter{
		Page:          req.GetPage(),
		PageSize:      req.GetPageSize(),
		Category:      req.GetCategory(),
		ComponentName: req.GetComponentName(),
		CoreHS6:       req.GetCoreHs6(),
		Enabled:       parseHsMetaEnabledQuery(req.GetEnabled()),
	}
	rows, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, kerrors.InternalServer("HS_META_LIST", err.Error())
	}
	items := make([]*v1.HsMetaListRow, 0, len(rows))
	for _, r := range rows {
		items = append(items, &v1.HsMetaListRow{
			Id:            int64(r.ID),
			Category:      r.Category,
			ComponentName: r.ComponentName,
			CoreHs6:       r.CoreHS6,
			Description:   r.Description,
			Enabled:       r.Enabled,
			SortOrder:     r.SortOrder,
			UpdatedAt:     r.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return &v1.HsMetaListReply{Items: items, Total: total}, nil
}

func (s *HsMetaService) CreateHsMeta(ctx context.Context, req *v1.HsMetaCreateRequest) (*v1.HsMetaMutationReply, error) {
	if s.repo == nil || !s.repo.DBOk() {
		return nil, kerrors.ServiceUnavailable("HS_META_DB_DISABLED", "database not configured")
	}
	core := strings.TrimSpace(req.GetCoreHs6())
	if err := biz.ValidateCoreHS6(core); err != nil {
		return nil, kerrors.BadRequest("HS_META_INVALID", err.Error())
	}
	if err := biz.ValidateHsMetaComponentName(req.GetComponentName()); err != nil {
		return nil, kerrors.BadRequest("HS_META_INVALID", err.Error())
	}
	cn := biz.NormalizeHsMetaText(req.GetComponentName())
	desc := strings.TrimSpace(req.GetDescription())
	if utf8.RuneCountInString(desc) > 512 {
		return nil, kerrors.BadRequest("HS_META_INVALID", "description 过长")
	}
	cat := strings.TrimSpace(req.GetCategory())
	if utf8.RuneCountInString(cat) > 64 {
		return nil, kerrors.BadRequest("HS_META_INVALID", "category 过长")
	}
	n, err := s.repo.CountByCoreAndComponent(ctx, core, cn, 0)
	if err != nil {
		return nil, kerrors.InternalServer("HS_META_DUP_CHECK", err.Error())
	}
	if n > 0 {
		return nil, kerrors.Conflict("HS_META_DUPLICATE", "相同 core_hs6 与 component_name 已存在")
	}
	row := &biz.HsMetaRecord{
		Category:      cat,
		ComponentName: cn,
		CoreHS6:       core,
		Description:   desc,
		Enabled:       req.GetEnabled(),
		SortOrder:     req.GetSortOrder(),
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, kerrors.InternalServer("HS_META_CREATE", err.Error())
	}
	return &v1.HsMetaMutationReply{Ok: "1"}, nil
}

func (s *HsMetaService) UpdateHsMeta(ctx context.Context, req *v1.HsMetaUpdateRequest) (*v1.HsMetaMutationReply, error) {
	if s.repo == nil || !s.repo.DBOk() {
		return nil, kerrors.ServiceUnavailable("HS_META_DB_DISABLED", "database not configured")
	}
	if req.GetId() <= 0 {
		return nil, kerrors.BadRequest("HS_META_INVALID", "id 无效")
	}
	core := strings.TrimSpace(req.GetCoreHs6())
	if err := biz.ValidateCoreHS6(core); err != nil {
		return nil, kerrors.BadRequest("HS_META_INVALID", err.Error())
	}
	if err := biz.ValidateHsMetaComponentName(req.GetComponentName()); err != nil {
		return nil, kerrors.BadRequest("HS_META_INVALID", err.Error())
	}
	cn := biz.NormalizeHsMetaText(req.GetComponentName())
	desc := strings.TrimSpace(req.GetDescription())
	if utf8.RuneCountInString(desc) > 512 {
		return nil, kerrors.BadRequest("HS_META_INVALID", "description 过长")
	}
	cat := strings.TrimSpace(req.GetCategory())
	if utf8.RuneCountInString(cat) > 64 {
		return nil, kerrors.BadRequest("HS_META_INVALID", "category 过长")
	}
	n, err := s.repo.CountByCoreAndComponent(ctx, core, cn, uint64(req.GetId()))
	if err != nil {
		return nil, kerrors.InternalServer("HS_META_DUP_CHECK", err.Error())
	}
	if n > 0 {
		return nil, kerrors.Conflict("HS_META_DUPLICATE", "相同 core_hs6 与 component_name 已存在")
	}
	row := &biz.HsMetaRecord{
		ID:            uint64(req.GetId()),
		Category:      cat,
		ComponentName: cn,
		CoreHS6:       core,
		Description:   desc,
		Enabled:       req.GetEnabled(),
		SortOrder:     req.GetSortOrder(),
	}
	if err := s.repo.Update(ctx, row); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, kerrors.NotFound("HS_META_NOT_FOUND", "记录不存在")
		}
		return nil, kerrors.InternalServer("HS_META_UPDATE", err.Error())
	}
	return &v1.HsMetaMutationReply{Ok: "1"}, nil
}

func (s *HsMetaService) DeleteHsMeta(ctx context.Context, req *v1.HsMetaDeleteRequest) (*v1.HsMetaMutationReply, error) {
	if s.repo == nil || !s.repo.DBOk() {
		return nil, kerrors.ServiceUnavailable("HS_META_DB_DISABLED", "database not configured")
	}
	if req.GetId() <= 0 {
		return nil, kerrors.BadRequest("HS_META_INVALID", "id 无效")
	}
	if err := s.repo.Delete(ctx, uint64(req.GetId())); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, kerrors.NotFound("HS_META_NOT_FOUND", "记录不存在")
		}
		return nil, kerrors.InternalServer("HS_META_DELETE", err.Error())
	}
	return &v1.HsMetaMutationReply{Ok: "1"}, nil
}
