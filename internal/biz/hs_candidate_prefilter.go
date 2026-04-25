package biz

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	// DefaultHsPrefilterTopN 默认预筛 TopN。
	DefaultHsPrefilterTopN = 30
	// HsPrefilterUnboundedCap 设计 §7：不在服务端对并集候选做行级 TopN 截断时的上限（防内存失控）。
	HsPrefilterUnboundedCap = 25000
)

// ErrHsPrefilterNoCandidates 预筛无候选，可供上层做回退策略。
var ErrHsPrefilterNoCandidates = errors.New("hs prefilter: no candidates")

// HsPrefilterEmptyError 预筛无候选的 typed error。
type HsPrefilterEmptyError struct {
	Input HsPrefilterInput
}

func (e *HsPrefilterEmptyError) Error() string {
	component := strings.TrimSpace(e.Input.ComponentName)
	tech := strings.TrimSpace(e.Input.TechCategory)
	return fmt.Sprintf("hs prefilter: no candidates for tech_category=%q component_name=%q", tech, component)
}

func (e *HsPrefilterEmptyError) Is(target error) bool {
	return target == ErrHsPrefilterNoCandidates
}

// HsCandidatePrefilter 按规则检索并产出 TopN 候选。
type HsCandidatePrefilter struct {
	repo HsItemQueryRepo
	topN int
}

func NewHsCandidatePrefilter(repo HsItemQueryRepo, topN int) *HsCandidatePrefilter {
	return &HsCandidatePrefilter{repo: repo, topN: topN}
}

func (p *HsCandidatePrefilter) Prefilter(ctx context.Context, input HsPrefilterInput) ([]HsItemCandidate, error) {
	if p == nil || p.repo == nil {
		return nil, errors.New("hs prefilter: repo not configured")
	}
	limit := p.topN
	if limit <= 0 {
		limit = DefaultHsPrefilterTopN
	}
	candidates, err := p.repo.QueryCandidatesByRules(ctx, input, limit)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, &HsPrefilterEmptyError{Input: input}
	}
	return candidates, nil
}
