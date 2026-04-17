package biz

import (
	"context"
	"testing"
	"time"
)

type metricPoint struct {
	name   string
	value  float64
	labels []string
}

type logEntry struct {
	event  string
	fields map[string]any
}

type captureObserver struct {
	metrics []metricPoint
	logs    []logEntry
}

func (c *captureObserver) RecordMetric(name string, value float64, labels ...string) {
	cp := make([]string, len(labels))
	copy(cp, labels)
	c.metrics = append(c.metrics, metricPoint{name: name, value: value, labels: cp})
}

func (c *captureObserver) EmitLog(event string, fields map[string]any) {
	cp := make(map[string]any, len(fields))
	for k, v := range fields {
		cp[k] = v
	}
	c.logs = append(c.logs, logEntry{event: event, fields: cp})
}

func TestObservability_EmitMetricsByStage(t *testing.T) {
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{}
	mapRepo := &spyMappingRepo{}
	obs := &captureObserver{}
	resolver := NewHsModelResolver(allowAllChecker{}).WithStateMachine(taskRepo, recoRepo, mapRepo).
		WithFeatureExtractor(stubFeatureExtractor{out: HsPrefilterInput{TechCategory: "ic"}}).
		WithCandidatePrefilter(stubCandidatePrefilter{out: []HsItemCandidate{{CodeTS: "1111222233", Score: 0.92}}}).
		WithCandidateRecommender(stubCandidateRecommender{
			out: []HsItemCandidate{{CodeTS: "1111222233", Score: 0.92}},
		}).
		WithRunIDGenerator(func() string { return "run-obs-1" }).
		WithObserver(obs)

	_, err := resolver.ResolveByModel(context.Background(), HsModelResolveRequest{
		Model: "OBS-M1", Manufacturer: "OBS-MFR", RequestTraceID: "obs-trace-1",
		DatasheetCands: []HsDatasheetCandidate{{ID: 1, DatasheetURL: "https://x/obs.pdf", UpdatedAt: time.Now()}},
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	var hasTotal, hasLatency, hasAutoAccept bool
	for _, m := range obs.metrics {
		if m.name == "hs_resolve_total" {
			hasTotal = true
		}
		if m.name == "hs_resolve_stage_latency_ms" {
			hasLatency = true
		}
		if m.name == "hs_resolve_auto_accept_ratio" {
			hasAutoAccept = true
		}
	}
	if !hasTotal || !hasLatency || !hasAutoAccept {
		t.Fatalf("expected core metrics emitted, got total=%v latency=%v auto_accept=%v metrics=%+v", hasTotal, hasLatency, hasAutoAccept, obs.metrics)
	}

	confirmRepo := newInMemoryHsModelConfirmRepo()
	confirmer := NewHsModelConfirmService(taskRepo, recoRepo, mapRepo, confirmRepo).WithObserver(obs)
	_, err = confirmer.Confirm(context.Background(), HsModelConfirmRequest{
		RunID: "run-obs-1", CandidateRank: 1, ExpectedCodeTS: "1111222233", ConfirmRequestID: "obs-confirm-1",
	})
	if err != nil {
		t.Fatalf("confirm failed: %v", err)
	}
	hasManual := false
	for _, m := range obs.metrics {
		if m.name == "hs_resolve_manual_override_total" {
			hasManual = true
			break
		}
	}
	if !hasManual {
		t.Fatalf("expected hs_resolve_manual_override_total metric, got metrics=%+v", obs.metrics)
	}
}

func TestObservability_LogFieldsContainRequiredKeys(t *testing.T) {
	taskRepo := newInMemoryHsModelTaskRepo()
	recoRepo := &spyRecommendationRepo{}
	mapRepo := &spyMappingRepo{}
	obs := &captureObserver{}
	resolver := NewHsModelResolver(allowAllChecker{}).WithStateMachine(taskRepo, recoRepo, mapRepo).
		WithFeatureExtractor(stubFeatureExtractor{out: HsPrefilterInput{TechCategory: "ic"}}).
		WithCandidatePrefilter(stubCandidatePrefilter{out: []HsItemCandidate{
			{CodeTS: "1111222233", Score: 0.92},
			{CodeTS: "2222333344", Score: 0.88},
		}}).
		WithCandidateRecommender(stubCandidateRecommender{
			out: []HsItemCandidate{
				{CodeTS: "1111222233", Score: 0.92},
				{CodeTS: "2222333344", Score: 0.88},
			},
		}).
		WithRunIDGenerator(func() string { return "run-obs-2" }).
		WithObserver(obs)

	_, err := resolver.ResolveByModel(context.Background(), HsModelResolveRequest{
		Model: "OBS-M2", Manufacturer: "OBS-MFR", RequestTraceID: "obs-trace-2",
		DatasheetCands: []HsDatasheetCandidate{{ID: 2, DatasheetURL: "https://x/obs2.pdf", UpdatedAt: time.Now()}},
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if len(obs.logs) == 0 {
		t.Fatal("expected observability logs")
	}

	last := obs.logs[len(obs.logs)-1].fields
	required := []string{
		"model", "manufacturer", "task_id", "run_id", "stage", "datasheet_url",
		"datasheet_path", "extract_model", "recommend_model", "candidate_count",
		"best_score", "final_status", "error_code",
	}
	for _, key := range required {
		if _, ok := last[key]; !ok {
			t.Fatalf("missing required log field %q in %+v", key, last)
		}
	}
}
