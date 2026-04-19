package service

import (
	"context"
	"strings"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ListManufacturerCanonicals 返回 t_bom_manufacturer_alias 中已出现的 distinct canonical_id（供配单页下拉）。
func (s *BomService) ListManufacturerCanonicals(ctx context.Context, req *v1.ListManufacturerCanonicalsRequest) (*v1.ListManufacturerCanonicalsReply, error) {
	if s.alias == nil || !s.alias.DBOk() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	lim := int(req.GetLimit())
	rows, err := s.alias.ListDistinctCanonicals(ctx, lim)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.BomManufacturerCanonicalRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, &v1.BomManufacturerCanonicalRow{
			CanonicalId: r.CanonicalID,
			DisplayName: r.DisplayName,
		})
	}
	return &v1.ListManufacturerCanonicalsReply{Items: out}, nil
}

// CreateManufacturerAlias 写入一条厂牌别名；alias_norm 与配单一致使用 biz.NormalizeMfrString。
func (s *BomService) CreateManufacturerAlias(ctx context.Context, req *v1.CreateManufacturerAliasRequest) (*v1.CreateManufacturerAliasReply, error) {
	if s.alias == nil || !s.alias.DBOk() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	alias := strings.TrimSpace(req.GetAlias())
	canonicalID := strings.TrimSpace(req.GetCanonicalId())
	displayName := strings.TrimSpace(req.GetDisplayName())
	if alias == "" || canonicalID == "" || displayName == "" {
		return nil, kerrors.BadRequest("BAD_INPUT", "alias, canonical_id, display_name required")
	}
	aliasNorm := biz.NormalizeMfrString(alias)
	if aliasNorm == "" {
		return nil, kerrors.BadRequest("BAD_ALIAS", "alias normalizes to empty")
	}
	err := s.alias.CreateRow(ctx, canonicalID, displayName, alias, aliasNorm)
	if err == nil {
		return &v1.CreateManufacturerAliasReply{}, nil
	}
	low := strings.ToLower(err.Error())
	if strings.Contains(low, "duplicate") {
		return nil, kerrors.Conflict("ALIAS_EXISTS", "alias_norm already exists")
	}
	return nil, err
}
