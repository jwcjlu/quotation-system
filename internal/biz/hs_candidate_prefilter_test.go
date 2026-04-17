package biz

import (
	"context"
	"errors"
	"testing"
)

type stubHsItemQueryRepo struct {
	candidates []HsItemCandidate
	limitSeen  int
	err        error
}

func (s *stubHsItemQueryRepo) DBOk() bool { return true }

func (s *stubHsItemQueryRepo) QueryCandidatesByRules(_ context.Context, _ HsPrefilterInput, limit int) ([]HsItemCandidate, error) {
	s.limitSeen = limit
	if s.err != nil {
		return nil, s.err
	}
	return append([]HsItemCandidate(nil), s.candidates...), nil
}

func TestPrefilter_ReturnTopNByRules(t *testing.T) {
	repo := &stubHsItemQueryRepo{
		candidates: []HsItemCandidate{
			{CodeTS: "8542310001", GName: "MCU"},
			{CodeTS: "8542310002", GName: "MCU Pro"},
		},
	}
	engine := NewHsCandidatePrefilter(repo, 0)

	got, err := engine.Prefilter(context.Background(), HsPrefilterInput{
		TechCategory:  "集成电路",
		ComponentName: "MCU",
		PackageForm:   "QFN",
		KeySpecs: map[string]string{
			"voltage": "3.3V",
		},
	})
	if err != nil {
		t.Fatalf("prefilter error: %v", err)
	}
	if repo.limitSeen != DefaultHsPrefilterTopN {
		t.Fatalf("expected default topN=%d, got %d", DefaultHsPrefilterTopN, repo.limitSeen)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(got))
	}
	if got[0].CodeTS != "8542310001" {
		t.Fatalf("unexpected rank1 candidate: %+v", got[0])
	}
}

func TestPrefilter_EmptyCandidates(t *testing.T) {
	engine := NewHsCandidatePrefilter(&stubHsItemQueryRepo{}, 25)

	got, err := engine.Prefilter(context.Background(), HsPrefilterInput{
		TechCategory:  "集成电路",
		ComponentName: "MCU",
	})
	if err == nil {
		t.Fatalf("expected typed no-candidate error, got nil with candidates=%v", got)
	}
	if !errors.Is(err, ErrHsPrefilterNoCandidates) {
		t.Fatalf("expected ErrHsPrefilterNoCandidates, got: %v", err)
	}
	var typedErr *HsPrefilterEmptyError
	if !errors.As(err, &typedErr) {
		t.Fatalf("expected *HsPrefilterEmptyError, got: %T", err)
	}
}

func TestPrefilter_SynonymRecallPathFromRepo(t *testing.T) {
	repo := &stubHsItemQueryRepo{
		candidates: []HsItemCandidate{
			{
				CodeTS: "8542310010",
				GName:  "Microcontroller Unit QFN",
				ScoreDetail: HsPrefilterScoreDetail{
					ComponentNameMatched: true,
				},
			},
		},
	}
	engine := NewHsCandidatePrefilter(repo, 10)
	got, err := engine.Prefilter(context.Background(), HsPrefilterInput{
		TechCategory:  "集成电路",
		ComponentName: "单片机",
	})
	if err != nil {
		t.Fatalf("prefilter synonym recall should not error, got: %v", err)
	}
	if len(got) != 1 || got[0].CodeTS != "8542310010" {
		t.Fatalf("expected recalled candidate from synonym path, got %+v", got)
	}
	if !got[0].ScoreDetail.ComponentNameMatched {
		t.Fatalf("expected component matched detail, got %+v", got[0].ScoreDetail)
	}
}
