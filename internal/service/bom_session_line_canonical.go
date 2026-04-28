package service

import (
	"context"
	"strings"

	"caichip/internal/biz"
)

func (s *BomService) canonicalPtrForManufacturer(ctx context.Context, raw string) (*string, error) {
	if strings.TrimSpace(raw) == "" || s == nil || s.alias == nil {
		return nil, nil
	}
	id, hit, err := biz.ResolveManufacturerCanonical(ctx, raw, s.alias)
	if err != nil {
		return nil, err
	}
	if !hit {
		return nil, nil
	}
	return &id, nil
}

func (s *BomService) canonicalizeBomImportLines(ctx context.Context, lines []biz.BomImportLine) ([]biz.BomImportLine, error) {
	out := make([]biz.BomImportLine, len(lines))
	copy(out, lines)
	for i := range out {
		canon, err := s.canonicalPtrForManufacturer(ctx, out[i].Mfr)
		if err != nil {
			return nil, err
		}
		out[i].ManufacturerCanonicalID = canon
	}
	return out, nil
}
