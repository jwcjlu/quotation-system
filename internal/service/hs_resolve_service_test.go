package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "caichip/api/bom/v1"
	"caichip/internal/biz"
	"caichip/internal/conf"
	"caichip/internal/data"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

type stubResolveRunner struct {
	task    *biz.HsModelTaskRecord
	err     error
	wait    time.Duration
	lastReq biz.HsModelResolveRequest
}

func (s *stubResolveRunner) ResolveByModel(_ context.Context, req biz.HsModelResolveRequest) (*biz.HsModelTaskRecord, error) {
	s.lastReq = req
	if s.wait > 0 {
		time.Sleep(s.wait)
	}
	if s.task != nil {
		cp := *s.task
		if cp.RunID == "" || cp.RunID == "run-ignored" {
			cp.RunID = req.RunID
		}
		return &cp, s.err
	}
	return s.task, s.err
}

type stubTaskQuery struct {
	task *biz.HsModelTaskRecord
	err  error
}

func (s *stubTaskQuery) GetByRunID(_ context.Context, _ string) (*biz.HsModelTaskRecord, error) {
	return s.task, s.err
}

func (s *stubTaskQuery) GetLatestByModelManufacturer(_ context.Context, _, _ string) (*biz.HsModelTaskRecord, error) {
	return s.task, s.err
}

type stubRecoRepo struct{}

func (s *stubRecoRepo) ListByRunID(_ context.Context, _ string) ([]biz.HsModelRecommendationRecord, error) {
	return []biz.HsModelRecommendationRecord{
		{RunID: "run-1", CandidateRank: 1, CodeTS: "1234567890", Score: 0.99, Reason: "top1"},
	}, nil
}

func (s *stubRecoRepo) ListPendingReviews(_ context.Context, _ int, _ int, _ string, _ string) ([]biz.HsPendingReviewRecord, int, error) {
	return nil, 0, nil
}

type stubConfirmer struct {
	res *biz.HsModelConfirmResult
	err error
}

func (s *stubConfirmer) Confirm(_ context.Context, _ biz.HsModelConfirmRequest) (*biz.HsModelConfirmResult, error) {
	return s.res, s.err
}

type stubDatasheetSource struct {
	asset *biz.HsDatasheetAssetRecord
	err   error
}

type captureLogger struct {
	mu      sync.Mutex
	entries [][]any
}

func (l *captureLogger) Log(_ log.Level, keyvals ...any) error {
	row := append([]any(nil), keyvals...)
	l.mu.Lock()
	l.entries = append(l.entries, row)
	l.mu.Unlock()
	return nil
}

func keyvalsToMap(keyvals []any) map[string]any {
	out := make(map[string]any, len(keyvals)/2)
	for i := 0; i+1 < len(keyvals); i += 2 {
		k, ok := keyvals[i].(string)
		if !ok {
			continue
		}
		out[k] = keyvals[i+1]
	}
	return out
}

func findEventInKeyvals(keyvals []any) string {
	for i := 0; i+1 < len(keyvals); i += 2 {
		k, ok := keyvals[i].(string)
		if !ok || k != "event" {
			continue
		}
		if v, ok := keyvals[i+1].(string); ok {
			return v
		}
	}
	return ""
}

func (l *captureLogger) findByEvent(event string) map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := len(l.entries) - 1; i >= 0; i-- {
		if findEventInKeyvals(l.entries[i]) == event {
			return keyvalsToMap(l.entries[i])
		}
	}
	return nil
}

func (s *stubDatasheetSource) GetLatestByModelManufacturer(_ context.Context, _, _ string) (*biz.HsDatasheetAssetRecord, error) {
	return s.asset, s.err
}

func (s *stubDatasheetSource) ListQuoteDatasheetCandidates(ctx context.Context, model, manufacturer string) ([]biz.HsDatasheetCandidate, error) {
	a, err := s.GetLatestByModelManufacturer(ctx, model, manufacturer)
	if err != nil || a == nil || strings.TrimSpace(a.DatasheetURL) == "" {
		return nil, err
	}
	return []biz.HsDatasheetCandidate{
		{ID: a.ID, DatasheetURL: strings.TrimSpace(a.DatasheetURL), UpdatedAt: a.UpdatedAt},
	}, nil
}

func defaultDatasheetSource() *stubDatasheetSource {
	return &stubDatasheetSource{
		asset: &biz.HsDatasheetAssetRecord{
			ID:           1,
			DatasheetURL: "https://example.com/default.pdf",
			UpdatedAt:    time.Now(),
		},
	}
}

func TestHsResolveService_Return202WhenTimeout(t *testing.T) {
	svc := NewHsResolveService(&stubResolveRunner{
		task: &biz.HsModelTaskRecord{RunID: "run-timeout", TaskStatus: biz.HsTaskStatusRunning, ResultStatus: biz.HsResultStatusPendingReview},
		wait: 50 * time.Millisecond,
	}, &stubTaskQuery{}, &stubRecoRepo{}, defaultDatasheetSource(), nil, 1*time.Millisecond)

	resp, err := svc.ResolveByModel(context.Background(), &v1.HsResolveByModelRequest{
		Model:          "STM32F103",
		Manufacturer:   "ST",
		RequestTraceId: "trace-timeout",
	})
	if err != nil {
		t.Fatalf("ResolveByModel error: %v", err)
	}
	if !resp.GetAccepted() || resp.GetTaskId() == "" {
		t.Fatalf("expected accepted async response with task_id, got %#v", resp)
	}
}

func TestHsResolveService_Return200WhenCompleted(t *testing.T) {
	svc := NewHsResolveService(&stubResolveRunner{
		task: &biz.HsModelTaskRecord{
			RunID:        "run-ok",
			TaskStatus:   biz.HsTaskStatusSuccess,
			ResultStatus: biz.HsResultStatusConfirmed,
			BestCodeTS:   "1234567890",
			BestScore:    0.98,
		},
	}, &stubTaskQuery{}, &stubRecoRepo{}, defaultDatasheetSource(), nil, time.Second)

	resp, err := svc.ResolveByModel(context.Background(), &v1.HsResolveByModelRequest{
		Model:          "STM32F103",
		Manufacturer:   "ST",
		RequestTraceId: "trace-ok",
	})
	if err != nil {
		t.Fatalf("ResolveByModel error: %v", err)
	}
	if resp.GetAccepted() {
		t.Fatalf("expected sync completed response, got accepted=true")
	}
	if resp.GetRunId() != "run-ok" || resp.GetTaskStatus() != biz.HsTaskStatusSuccess || resp.GetResultStatus() != biz.HsResultStatusConfirmed {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestHsResolveService_ResponseFields(t *testing.T) {
	runner := &stubResolveRunner{
		task: &biz.HsModelTaskRecord{
			RunID:        "run-fields",
			TaskStatus:   biz.HsTaskStatusSuccess,
			ResultStatus: biz.HsResultStatusPendingReview,
			BestCodeTS:   "1234567890",
			BestScore:    0.91,
		},
	}
	svc := NewHsResolveService(runner, &stubTaskQuery{}, &stubRecoRepo{}, defaultDatasheetSource(), nil, time.Second)
	resp, err := svc.ResolveByModel(context.Background(), &v1.HsResolveByModelRequest{
		Model:          "STM32F103",
		Manufacturer:   "ST",
		RequestTraceId: "trace-fields",
	})
	if err != nil {
		t.Fatalf("ResolveByModel error: %v", err)
	}
	if resp.GetRunId() == "" || resp.GetDecisionMode() == "" || resp.GetTaskStatus() == "" || resp.GetResultStatus() == "" {
		t.Fatalf("response fields missing: %#v", resp)
	}
}

func TestHsResolveService_RequireRequestTraceID(t *testing.T) {
	svc := NewHsResolveService(&stubResolveRunner{}, &stubTaskQuery{}, &stubRecoRepo{}, nil, nil, time.Second)
	_, err := svc.ResolveByModel(context.Background(), &v1.HsResolveByModelRequest{
		Model:        "STM32F103",
		Manufacturer: "ST",
	})
	if err == nil {
		t.Fatal("expected validation error when request_trace_id is missing")
	}
}

func TestHsResolveService_AllowsEmptyManufacturer(t *testing.T) {
	svc := NewHsResolveService(&stubResolveRunner{
		task: &biz.HsModelTaskRecord{
			RunID:        "run-empty-mfr",
			TaskStatus:   biz.HsTaskStatusSuccess,
			ResultStatus: biz.HsResultStatusConfirmed,
			BestCodeTS:   "1234567890",
			BestScore:    0.9,
		},
	}, &stubTaskQuery{}, &stubRecoRepo{}, defaultDatasheetSource(), nil, time.Second)
	resp, err := svc.ResolveByModel(context.Background(), &v1.HsResolveByModelRequest{
		Model:          "PART-A",
		Manufacturer:   "",
		RequestTraceId: "trace-empty-mfr",
	})
	if err != nil {
		t.Fatalf("ResolveByModel: %v", err)
	}
	if resp.GetBestCodeTs() != "1234567890" {
		t.Fatalf("unexpected reply: %#v", resp)
	}
}

func TestHsResolveService_GetResolveTaskFailedPayload(t *testing.T) {
	svc := NewHsResolveService(
		&stubResolveRunner{},
		&stubTaskQuery{task: &biz.HsModelTaskRecord{
			RunID:        "run-failed",
			TaskStatus:   biz.HsTaskStatusFailed,
			ResultStatus: biz.HsResultStatusRejected,
			LastError:    "extract failed",
		}},
		&stubRecoRepo{},
		nil,
		nil,
		time.Second,
	)
	resp, err := svc.GetResolveTask(context.Background(), &v1.HsResolveTaskRequest{TaskId: "run-failed"})
	if err != nil {
		t.Fatalf("GetResolveTask error: %v", err)
	}
	if resp.GetTaskStatus() != biz.HsTaskStatusFailed || resp.GetErrorCode() == "" || resp.GetErrorMessage() == "" {
		t.Fatalf("expected failed payload with error fields, got %#v", resp)
	}
}

func TestHsResolveService_ConfirmResolve(t *testing.T) {
	svc := NewHsResolveService(
		&stubResolveRunner{},
		&stubTaskQuery{task: &biz.HsModelTaskRecord{
			RunID:        "run-1",
			Model:        "STM32F103",
			Manufacturer: "ST",
		}},
		&stubRecoRepo{},
		nil,
		&stubConfirmer{res: &biz.HsModelConfirmResult{
			RunID:            "run-1",
			CandidateRank:    1,
			CodeTS:           "1234567890",
			ConfirmRequestID: "confirm-1",
		}},
		time.Second,
	)
	resp, err := svc.ConfirmResolve(context.Background(), &v1.HsResolveConfirmRequest{
		Model:            "STM32F103",
		Manufacturer:     "ST",
		RunId:            "run-1",
		CandidateRank:    1,
		ExpectedCodeTs:   "1234567890",
		ConfirmRequestId: "confirm-1",
	})
	if err != nil {
		t.Fatalf("ConfirmResolve error: %v", err)
	}
	if resp.GetRunId() != "run-1" || resp.GetResultStatus() != biz.HsResultStatusConfirmed {
		t.Fatalf("unexpected confirm response: %#v", resp)
	}
}

func TestHsResolveService_ConfirmResolveAllowsEmptyManufacturer(t *testing.T) {
	svc := NewHsResolveService(
		&stubResolveRunner{},
		&stubTaskQuery{task: &biz.HsModelTaskRecord{
			RunID:        "run-empty-mfr",
			Model:        "PART-X",
			Manufacturer: "",
		}},
		&stubRecoRepo{},
		nil,
		&stubConfirmer{res: &biz.HsModelConfirmResult{
			RunID:            "run-empty-mfr",
			CandidateRank:    1,
			CodeTS:           "1234567890",
			ConfirmRequestID: "confirm-empty-mfr",
		}},
		time.Second,
	)
	resp, err := svc.ConfirmResolve(context.Background(), &v1.HsResolveConfirmRequest{
		Model:            "PART-X",
		Manufacturer:     "",
		RunId:            "run-empty-mfr",
		CandidateRank:    1,
		ExpectedCodeTs:   "1234567890",
		ConfirmRequestId: "confirm-empty-mfr",
	})
	if err != nil {
		t.Fatalf("ConfirmResolve error: %v", err)
	}
	if resp.GetRunId() != "run-empty-mfr" || resp.GetResultStatus() != biz.HsResultStatusConfirmed {
		t.Fatalf("unexpected confirm response: %#v", resp)
	}
}

func TestHsResolveService_GetResolveHistory(t *testing.T) {
	svc := NewHsResolveService(
		&stubResolveRunner{},
		&stubTaskQuery{task: &biz.HsModelTaskRecord{
			RunID:        "run-1",
			TaskStatus:   biz.HsTaskStatusSuccess,
			ResultStatus: biz.HsResultStatusPendingReview,
			BestCodeTS:   "1234567890",
			BestScore:    0.88,
		}},
		&stubRecoRepo{},
		nil,
		nil,
		time.Second,
	)
	resp, err := svc.GetResolveHistory(context.Background(), &v1.HsResolveHistoryRequest{
		Model:        "STM32F103",
		Manufacturer: "ST",
	})
	if err != nil {
		t.Fatalf("GetResolveHistory error: %v", err)
	}
	if len(resp.GetItems()) != 1 || resp.GetItems()[0].GetRunId() != "run-1" {
		t.Fatalf("unexpected history response: %#v", resp)
	}
}

func TestHsResolveService_ForceRefreshBypassesMappingCache(t *testing.T) {
	runner := &stubResolveRunner{
		task: &biz.HsModelTaskRecord{
			RunID:        "run-force",
			TaskStatus:   biz.HsTaskStatusSuccess,
			ResultStatus: biz.HsResultStatusConfirmed,
		},
	}
	svc := NewHsResolveService(runner, &stubTaskQuery{}, &stubRecoRepo{}, defaultDatasheetSource(), nil, time.Second)
	_, err := svc.ResolveByModel(context.Background(), &v1.HsResolveByModelRequest{
		Model:          "STM32F103",
		Manufacturer:   "ST",
		RequestTraceId: "trace-force",
		ForceRefresh:   true,
	})
	if err != nil {
		t.Fatalf("ResolveByModel error: %v", err)
	}
	if !runner.lastReq.ForceRefresh {
		t.Fatal("expected force_refresh to be passed into biz request")
	}
	base := svc.makeTaskID("STM32F103", "ST", "trace-force")
	if runner.lastReq.RunID == base {
		t.Fatalf("expected force_refresh run_id to differ from base id, got %q", runner.lastReq.RunID)
	}
	if !strings.HasPrefix(runner.lastReq.RunID, base+"|refresh-") {
		t.Fatalf("expected force_refresh run_id with refresh suffix, got %q", runner.lastReq.RunID)
	}
}

func TestHsResolveService_TaskIDEqualsRunID(t *testing.T) {
	runner := &stubResolveRunner{
		task: &biz.HsModelTaskRecord{
			RunID:        "run-ignored",
			TaskStatus:   biz.HsTaskStatusSuccess,
			ResultStatus: biz.HsResultStatusConfirmed,
		},
	}
	svc := NewHsResolveService(runner, &stubTaskQuery{}, &stubRecoRepo{}, defaultDatasheetSource(), nil, time.Second)
	resp, err := svc.ResolveByModel(context.Background(), &v1.HsResolveByModelRequest{
		Model:          "STM32F103",
		Manufacturer:   "ST",
		RequestTraceId: "trace-taskid",
	})
	if err != nil {
		t.Fatalf("ResolveByModel error: %v", err)
	}
	if resp.GetTaskId() == "" || resp.GetRunId() == "" {
		t.Fatalf("expected task_id and run_id not empty: %#v", resp)
	}
	if resp.GetTaskId() != resp.GetRunId() || runner.lastReq.RunID != resp.GetTaskId() {
		t.Fatalf("expected task_id == run_id == request.run_id, got resp=%#v, req=%#v", resp, runner.lastReq)
	}
}

func TestHsResolveService_UseRepoDatasheetCandidates(t *testing.T) {
	runner := &stubResolveRunner{
		task: &biz.HsModelTaskRecord{
			RunID:        "run-ds",
			TaskStatus:   biz.HsTaskStatusSuccess,
			ResultStatus: biz.HsResultStatusConfirmed,
		},
	}
	ds := &stubDatasheetSource{
		asset: &biz.HsDatasheetAssetRecord{
			ID:           11,
			DatasheetURL: "https://example.com/ds.pdf",
			UpdatedAt:    time.Now(),
		},
	}
	svc := NewHsResolveService(runner, &stubTaskQuery{}, &stubRecoRepo{}, ds, nil, time.Second)
	_, err := svc.ResolveByModel(context.Background(), &v1.HsResolveByModelRequest{
		Model:          "STM32F103",
		Manufacturer:   "ST",
		RequestTraceId: "trace-ds",
	})
	if err != nil {
		t.Fatalf("ResolveByModel error: %v", err)
	}
	if len(runner.lastReq.DatasheetCands) != 1 || runner.lastReq.DatasheetCands[0].DatasheetURL != "https://example.com/ds.pdf" {
		t.Fatalf("expected datasheet candidates from repo, got %#v", runner.lastReq.DatasheetCands)
	}
}

func TestNewDefaultHsResolveService_DisabledWhenDependencyMissing(t *testing.T) {
	svc := NewDefaultHsResolveService(
		&conf.Bootstrap{},
		log.NewStdLogger(nil),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if svc == nil {
		t.Fatal("expected service instance")
	}
	if svc.resolver != nil {
		t.Fatal("expected resolver to stay nil when dependencies are missing")
	}
}

func TestNewDefaultHsResolveService_EnableResolverWhenDependenciesProvided(t *testing.T) {
	cfg := &conf.Bootstrap{
		Openai: &conf.OpenAI{
			ApiKey: "test-key",
			Model:  "gpt-4o-mini",
		},
	}
	openAI := data.NewOpenAIChat(cfg)
	if openAI == nil {
		t.Fatal("expected openai client")
	}
	d := &data.Data{}
	svc := NewDefaultHsResolveService(
		cfg,
		log.NewStdLogger(nil),
		data.NewHsModelMappingRepo(d, nil),
		data.NewHsDatasheetAssetRepo(d),
		data.NewHsModelRecommendationRepo(d),
		data.NewHsItemQueryRepo(d),
		data.NewHsModelTaskRepo(d),
		openAI,
		data.NewHsModelFeaturesRepo(d),
		nil,
		nil,
	)
	if svc == nil {
		t.Fatal("expected service instance")
	}
	if svc.resolver == nil {
		t.Fatal("expected resolver to be configured with full dependencies")
	}
}

func TestHsResolveService_LogFieldsForResolveReply(t *testing.T) {
	runner := &stubResolveRunner{
		task: &biz.HsModelTaskRecord{
			RunID:        "run-log-reply",
			TaskStatus:   biz.HsTaskStatusSuccess,
			ResultStatus: biz.HsResultStatusConfirmed,
		},
	}
	svc := NewHsResolveService(runner, &stubTaskQuery{}, &stubRecoRepo{}, defaultDatasheetSource(), nil, time.Second)
	logSink := &captureLogger{}
	svc.log = log.NewHelper(logSink)

	_, err := svc.ResolveByModel(context.Background(), &v1.HsResolveByModelRequest{
		Model:          "STM32F103",
		Manufacturer:   "ST",
		RequestTraceId: "trace-log-reply",
	})
	if err != nil {
		t.Fatalf("ResolveByModel error: %v", err)
	}
	got := logSink.findByEvent("resolve.reply")
	if got == nil {
		t.Fatal("expected resolve.reply log entry")
	}
	required := []string{"event", "model", "manufacturer", "task_id", "run_id", "stage", "final_status", "error_code"}
	for _, key := range required {
		if _, ok := got[key]; !ok {
			t.Fatalf("resolve.reply missing key %q, got=%+v", key, got)
		}
	}
}

func TestHsResolveService_LogFieldsForResolveAcceptedAsync(t *testing.T) {
	svc := NewHsResolveService(&stubResolveRunner{
		task: &biz.HsModelTaskRecord{RunID: "run-log-async", TaskStatus: biz.HsTaskStatusRunning, ResultStatus: biz.HsResultStatusPendingReview},
		wait: 60 * time.Millisecond,
	}, &stubTaskQuery{}, &stubRecoRepo{}, defaultDatasheetSource(), nil, 1*time.Millisecond)
	logSink := &captureLogger{}
	svc.log = log.NewHelper(logSink)

	_, err := svc.ResolveByModel(context.Background(), &v1.HsResolveByModelRequest{
		Model:          "STM32F103",
		Manufacturer:   "ST",
		RequestTraceId: "trace-log-async",
	})
	if err != nil {
		t.Fatalf("ResolveByModel error: %v", err)
	}
	got := logSink.findByEvent("resolve.accepted_async")
	if got == nil {
		t.Fatal("expected resolve.accepted_async log entry")
	}
	required := []string{"event", "model", "manufacturer", "task_id", "run_id", "stage", "final_status", "error_code"}
	for _, key := range required {
		if _, ok := got[key]; !ok {
			t.Fatalf("resolve.accepted_async missing key %q, got=%+v", key, got)
		}
	}
}

func TestManualRunFingerprintChangesWithInputs(t *testing.T) {
	a := manualRunFingerprint("x", "")
	b := manualRunFingerprint("y", "")
	if a == b {
		t.Fatalf("expected different fingerprints, got %q", a)
	}
	if manualRunFingerprint("", "") != "" {
		t.Fatal("expected empty fingerprint when both empty")
	}
}

func TestHsResolveService_EarlyRejectNoDatasheetNoManual(t *testing.T) {
	svc := NewHsResolveService(&stubResolveRunner{
		task: &biz.HsModelTaskRecord{RunID: "run-x", TaskStatus: biz.HsTaskStatusSuccess},
	}, &stubTaskQuery{}, &stubRecoRepo{}, &stubDatasheetSource{asset: nil}, nil, time.Second)
	svc.attachManualUpload(nil, biz.NewHsResolveConfig(nil), "")
	_, err := svc.ResolveByModel(context.Background(), &v1.HsResolveByModelRequest{
		Model:          "STM32",
		RequestTraceId: "trace-no-ds",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if se := kerrors.FromError(err); se == nil || se.Code != 400 {
		t.Fatalf("expected http 400 style error, got %v", err)
	}
	if !strings.Contains(err.Error(), "DATASHEET_OR_MANUAL_REQUIRED") {
		t.Fatalf("expected DATASHEET_OR_MANUAL_REQUIRED in message, got %v", err)
	}
}
