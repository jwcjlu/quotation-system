package biz

import (
	"context"
	"encoding/json"
	"strings"
)

// persistHsModelFeatures 在推荐前将抽取特征写入 t_hs_model_features；命中别名时写入 manufacturer_canonical_id，未命中不传该字段（nil）。
func (r *HsModelResolver) persistHsModelFeatures(ctx context.Context, n HsModelResolveRequest, asset *HsDatasheetAssetRecord, input *HsPrefilterInput) error {
	if r == nil || r.featuresRepo == nil || !r.featuresRepo.DBOk() {
		return nil
	}
	if asset == nil || asset.ID == 0 || input == nil {
		return nil
	}
	mCanonPtr, err := r.manufacturerCanonicalPtr(ctx, n.Manufacturer)
	if err != nil {
		return err
	}
	rankedJSON, err := json.Marshal(input.TechCategoryRanked)
	if err != nil {
		return err
	}
	keySpecsJSON, err := json.Marshal(input.KeySpecs)
	if err != nil {
		return err
	}
	rawExtractJSON := []byte(`{}`)
	rec := &HsModelFeaturesRecord{
		Model:                   n.Model,
		Manufacturer:            n.Manufacturer,
		ManufacturerCanonicalID: mCanonPtr,
		AssetID:                 asset.ID,
		TechCategory:            strings.TrimSpace(input.TechCategory),
		TechCategoryRankedJSON:  rankedJSON,
		ComponentName:           strings.TrimSpace(input.ComponentName),
		PackageForm:             strings.TrimSpace(input.PackageForm),
		KeySpecsJSON:            keySpecsJSON,
		RawExtractJSON:          rawExtractJSON,
		ExtractModel:            strings.TrimSpace(n.RecommendModel),
		ExtractVersion:          strings.TrimSpace(n.FeaturesVersion),
	}
	_, err = r.featuresRepo.Create(ctx, rec)
	return err
}
