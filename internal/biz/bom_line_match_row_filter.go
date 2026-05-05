package biz

import (
	"context"
	"strings"
)

func lineMatchRowPasses(ctx context.Context, in LineMatchInput, bomCanonID string, bomMfrRequired bool, row AgentQuoteRow, alias AliasLookup) (bool, error) {
	if strings.TrimSpace(row.Model) == "" {
		return false, nil
	}
	if NormalizeMPNForBOMSearch(in.BomMpn) != NormalizeMPNForBOMSearch(row.Model) {
		return false, nil
	}
	if pkg := strings.TrimSpace(in.BomPackage); pkg != "" {
		if NormalizeMfrString(row.Package) != NormalizeMfrString(pkg) {
			return false, nil
		}
	}
	if !bomMfrRequired {
		return true, nil
	}
	if strings.TrimSpace(row.Manufacturer) == "" {
		return false, nil
	}
	st := strings.TrimSpace(row.ManufacturerReviewStatus)
	if st != "" {
		switch st {
		case MfrReviewPending, MfrReviewRejected:
			return false, nil
		case MfrReviewAccepted:
			qCanon := ""
			if row.ManufacturerCanonicalID != nil {
				qCanon = strings.TrimSpace(*row.ManufacturerCanonicalID)
			}
			if qCanon == "" || bomCanonID == "" {
				return false, nil
			}
			return qCanon == bomCanonID, nil
		default:
			return false, nil
		}
	}
	qCanon, qHit, err := ResolveManufacturerCanonical(ctx, row.Manufacturer, alias)
	if err != nil {
		return false, err
	}
	if !qHit || qCanon != bomCanonID {
		return false, nil
	}
	return true, nil
}

// quoteRowPassesModelAndPackage 仅校验型号与封装（若有），不含厂牌；用于识别「厂牌不匹配」类跳过。
func quoteRowPassesModelAndPackage(in LineMatchInput, row AgentQuoteRow) bool {
	if strings.TrimSpace(row.Model) == "" {
		return false
	}
	if NormalizeMPNForBOMSearch(in.BomMpn) != NormalizeMPNForBOMSearch(row.Model) {
		return false
	}
	if pkg := strings.TrimSpace(in.BomPackage); pkg != "" {
		if NormalizeMfrString(row.Package) != NormalizeMfrString(pkg) {
			return false
		}
	}
	return true
}
