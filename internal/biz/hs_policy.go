package biz

import (
	"sort"

	"caichip/pkg/hsclassifycodes"
)

type HSFinalDecisionEngine struct{}

func NewHSFinalDecisionEngine() *HSFinalDecisionEngine { return &HSFinalDecisionEngine{} }

func (e *HSFinalDecisionEngine) Decide(policy *HSClassifyPolicy, confidence, completeness, topGap float64, hardConflicts []hsclassifycodes.ReviewReasonCode, policySourceUnavailable bool) HSFinalSuggestion {
	reasons := make([]hsclassifycodes.ReviewReasonCode, 0, 4)
	reasons = append(reasons, hardConflicts...)
	if policySourceUnavailable {
		reasons = append(reasons, hsclassifycodes.ReviewReasonCodeFrPolicySourceUnavailable)
	}
	if confidence < policy.ForceReviewConfidenceMax {
		reasons = append(reasons, hsclassifycodes.ReviewReasonCodeFrLowConfidence)
	}
	if completeness < policy.ForceReviewCompleteness {
		reasons = append(reasons, hsclassifycodes.ReviewReasonCodeFrLowCompleteness)
	}
	if topGap < policy.QuickReviewTopGapMin {
		reasons = append(reasons, hsclassifycodes.ReviewReasonCodeQrTopGapLow)
	}
	if confidence < policy.AutoPassConfidenceMin && confidence >= policy.ForceReviewConfidenceMax {
		reasons = append(reasons, hsclassifycodes.ReviewReasonCodeQrConfidenceRange)
	}
	if completeness < policy.AutoPassCompletenessMin && completeness >= policy.ForceReviewCompleteness {
		reasons = append(reasons, hsclassifycodes.ReviewReasonCodeQrCompletenessRange)
	}
	reasons = uniqueAndSortReviewReasons(reasons)

	final := HSFinalSuggestion{Confidence: confidence}
	if len(reasons) == 0 && confidence >= policy.AutoPassConfidenceMin && completeness >= policy.AutoPassCompletenessMin && topGap >= policy.AutoPassTopGapMin {
		final.ReviewRequired = false
		return final
	}
	final.ReviewRequired = true
	for _, code := range reasons {
		final.ReviewReasonCodes = append(final.ReviewReasonCodes, string(code))
	}
	return final
}

func uniqueAndSortReviewReasons(in []hsclassifycodes.ReviewReasonCode) []hsclassifycodes.ReviewReasonCode {
	seen := map[hsclassifycodes.ReviewReasonCode]struct{}{}
	out := make([]hsclassifycodes.ReviewReasonCode, 0, len(in))
	for _, c := range in {
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	order := map[hsclassifycodes.ReviewReasonGroup]int{
		hsclassifycodes.ReviewReasonGroupHr: 0,
		hsclassifycodes.ReviewReasonGroupFr: 1,
		hsclassifycodes.ReviewReasonGroupQr: 2,
	}
	sort.Slice(out, func(i, j int) bool {
		gi := hsclassifycodes.ReviewReasonCodeGroup[out[i]]
		gj := hsclassifycodes.ReviewReasonCodeGroup[out[j]]
		if order[gi] != order[gj] {
			return order[gi] < order[gj]
		}
		return out[i] < out[j]
	})
	return out
}
