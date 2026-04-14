package biz

import (
	"testing"

	"caichip/pkg/hsclassifycodes"
)

func TestHsPolicy_AutoPass(t *testing.T) {
	p := &HSClassifyPolicy{AutoPassConfidenceMin: 85, AutoPassCompletenessMin: 0.9, AutoPassTopGapMin: 8, QuickReviewTopGapMin: 5, ForceReviewConfidenceMax: 70, ForceReviewCompleteness: 0.6}
	got := NewHSFinalDecisionEngine().Decide(p, 90, 1.0, 10, nil, false)
	if got.ReviewRequired {
		t.Fatalf("expect auto pass")
	}
}

func TestHsPolicy_QuickReview(t *testing.T) {
	p := &HSClassifyPolicy{AutoPassConfidenceMin: 85, AutoPassCompletenessMin: 0.9, AutoPassTopGapMin: 8, QuickReviewTopGapMin: 8, ForceReviewConfidenceMax: 70, ForceReviewCompleteness: 0.6}
	got := NewHSFinalDecisionEngine().Decide(p, 80, 0.95, 4, nil, false)
	if !got.ReviewRequired || len(got.ReviewReasonCodes) == 0 {
		t.Fatalf("expect quick review")
	}
}

func TestHsPolicy_ForceReviewPriority(t *testing.T) {
	p := &HSClassifyPolicy{AutoPassConfidenceMin: 85, AutoPassCompletenessMin: 0.9, AutoPassTopGapMin: 8, QuickReviewTopGapMin: 5, ForceReviewConfidenceMax: 70, ForceReviewCompleteness: 0.6}
	got := NewHSFinalDecisionEngine().Decide(p, 60, 0.5, 1, []hsclassifycodes.ReviewReasonCode{hsclassifycodes.ReviewReasonCodeHrMissingGlobalRequired}, true)
	if !got.ReviewRequired {
		t.Fatalf("expect force review")
	}
	if got.ReviewReasonCodes[0] != string(hsclassifycodes.ReviewReasonCodeHrMissingGlobalRequired) {
		t.Fatalf("expect HR reason first, got %v", got.ReviewReasonCodes)
	}
}
