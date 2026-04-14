package data

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"caichip/internal/biz"
)

type HSPolicyRepo struct {
	data *Data
}

func NewHSPolicyRepo(data *Data) *HSPolicyRepo { return &HSPolicyRepo{data: data} }
func (r *HSPolicyRepo) DBOk() bool             { return r != nil && r.data != nil && r.data.DB != nil }

func (r *HSPolicyRepo) LoadByDeclarationDate(ctx context.Context, declarationDate time.Time) (*biz.HSClassifyPolicy, bool, error) {
	if r.DBOk() {
		var row HsPolicyVersion
		err := r.data.DB.WithContext(ctx).
			Where("enabled = ? AND effective_from <= ?", true, declarationDate).
			Order("effective_from DESC").
			First(&row).Error
		if err == nil {
			return &biz.HSClassifyPolicy{
				VersionID:                row.VersionID,
				AutoPassConfidenceMin:    row.AutoPassConfidenceMin,
				AutoPassCompletenessMin:  row.AutoPassCompletenessMin,
				AutoPassTopGapMin:        row.AutoPassTopGapMin,
				QuickReviewTopGapMin:     row.QuickReviewTopGapMin,
				QuickReviewConfidenceMin: row.QuickReviewConfidenceMin,
				ForceReviewConfidenceMax: row.ForceReviewConfidenceMax,
				ForceReviewCompleteness:  row.ForceReviewCompleteness,
			}, false, nil
		}
	}
	p, err := loadPolicySnapshotFromFile()
	if err != nil {
		return nil, true, errors.Join(biz.ErrHSPolicySourceUnavailable, err)
	}
	return p, true, nil
}

type hsPolicySnapshot struct {
	Version     string           `json:"version"`
	AutoPass    hsThresholdBlock `json:"auto_pass"`
	QuickReview hsThresholdBlock `json:"quick_review"`
	ForceReview hsThresholdBlock `json:"force_review"`
}

type hsThresholdBlock struct {
	ConfidenceGte   float64 `json:"confidence_gte"`
	CompletenessGte float64 `json:"completeness_gte"`
	TopGapGte       float64 `json:"top_gap_gte"`
	TopGapLt        float64 `json:"top_gap_lt"`
	ConfidenceLt    float64 `json:"confidence_lt"`
	CompletenessLt  float64 `json:"completeness_lt"`
}

func loadPolicySnapshotFromFile() (*biz.HSClassifyPolicy, error) {
	root, err := findRepoRoot()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, "docs", "schema", "hs_final_decision_policy.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snap hsPolicySnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, err
	}
	return &biz.HSClassifyPolicy{
		VersionID:                snap.Version,
		AutoPassConfidenceMin:    snap.AutoPass.ConfidenceGte,
		AutoPassCompletenessMin:  snap.AutoPass.CompletenessGte,
		AutoPassTopGapMin:        snap.AutoPass.TopGapGte,
		QuickReviewTopGapMin:     snap.QuickReview.TopGapLt,
		QuickReviewConfidenceMin: snap.QuickReview.ConfidenceGte,
		ForceReviewConfidenceMax: snap.ForceReview.ConfidenceLt,
		ForceReviewCompleteness:  snap.ForceReview.CompletenessLt,
	}, nil
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return "", os.ErrNotExist
}
