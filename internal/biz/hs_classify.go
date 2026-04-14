package biz

import (
	"context"
	"errors"
	"strings"
	"time"

	"caichip/pkg/hsclassifycodes"

	"github.com/go-kratos/kratos/v2/log"
)

type HSClassifyUsecase struct {
	policyRepo HSPolicyRepo
	caseRepo   HSCaseRepo
	reviewRepo HSReviewRepo
	engine     *HSFinalDecisionEngine
	log        *log.Helper
}

func NewHSClassifyUsecase(policyRepo HSPolicyRepo, caseRepo HSCaseRepo, reviewRepo HSReviewRepo, engine *HSFinalDecisionEngine, logger log.Logger) *HSClassifyUsecase {
	return &HSClassifyUsecase{policyRepo: policyRepo, caseRepo: caseRepo, reviewRepo: reviewRepo, engine: engine, log: log.NewHelper(logger)}
}

func (uc *HSClassifyUsecase) ClassifyByModel(ctx context.Context, req *HSClassifyRequest) (*HSClassifyResult, error) {
	day, err := time.Parse("2006-01-02", strings.TrimSpace(req.DeclarationDate))
	if err != nil {
		return nil, errors.New("declaration_date format invalid, expected YYYY-MM-DD")
	}
	policy, policySourceUnavailable, err := uc.policyRepo.LoadByDeclarationDate(ctx, day)
	if err != nil {
		return nil, err
	}
	cases, err := uc.caseRepo.SearchTopCases(ctx, req, 5)
	if err != nil {
		return nil, err
	}
	result := &HSClassifyResult{Trace: HSClassifyTrace{PolicyVersionID: policy.VersionID, LLMVersion: "rule-only-mvp"}}
	for _, c := range cases {
		result.Candidates = append(result.Candidates, HSClassifyCandidate{
			HSCode:   c.HSCode,
			Score:    c.Score,
			Reason:   c.Reason,
			Evidence: append([]string(nil), c.Evidence...),
		})
		result.Trace.RetrievalRefs = append(result.Trace.RetrievalRefs, c.Title)
	}
	topConfidence, topGap := scoreStats(cases)
	completeness := estimateCompleteness(req)
	final := uc.engine.Decide(policy, topConfidence, completeness, topGap, hardConflicts(req), policySourceUnavailable)
	if policySourceUnavailable {
		result.Trace.RuleHits = append(result.Trace.RuleHits, "FR_POLICY_SOURCE_UNAVAILABLE")
	}
	if len(result.Candidates) > 0 {
		final.HSCode = result.Candidates[0].HSCode
	}
	result.FinalSuggestion = final
	_ = uc.reviewRepo.SaveDecision(ctx, HSReviewWrite{
		RequestKey:      req.Model + "|" + req.DeclarationDate,
		FinalHSCode:     final.HSCode,
		ReviewRequired:  final.ReviewRequired,
		ReviewReasons:   append([]string(nil), final.ReviewReasonCodes...),
		PolicyVersionID: policy.VersionID,
	})
	return result, nil
}

func estimateCompleteness(req *HSClassifyRequest) float64 {
	have := 0.0
	total := 4.0
	if strings.TrimSpace(req.Model) != "" {
		have++
	}
	if strings.TrimSpace(req.ProductNameCN) != "" {
		have++
	}
	if strings.TrimSpace(req.Manufacturer) != "" {
		have++
	}
	if strings.TrimSpace(req.Package) != "" {
		have++
	}
	return have / total
}

func scoreStats(cases []HSReferenceCase) (top float64, gap float64) {
	if len(cases) == 0 {
		return 0, 0
	}
	top = cases[0].Score
	if len(cases) == 1 {
		return top, top
	}
	return top, top - cases[1].Score
}

func hardConflicts(req *HSClassifyRequest) []hsclassifycodes.ReviewReasonCode {
	if strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.ProductNameCN) == "" {
		return []hsclassifycodes.ReviewReasonCode{hsclassifycodes.ReviewReasonCodeHrMissingGlobalRequired}
	}
	return nil
}
