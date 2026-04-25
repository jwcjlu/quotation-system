package biz

import "testing"

func TestBOMSearchTaskStatusStateMapping(t *testing.T) {
	tests := []struct {
		name       string
		state      string
		normalized string
		uiState    string
	}{
		{name: "pending", state: "pending", normalized: "pending", uiState: SearchTaskUIStatePending},
		{name: "retry backoff", state: "retry_backoff", normalized: "retry_backoff", uiState: SearchTaskUIStatePending},
		{name: "running", state: "running", normalized: "running", uiState: SearchTaskUIStateSearching},
		{name: "succeeded", state: "succeeded", normalized: "succeeded", uiState: SearchTaskUIStateSucceeded},
		{name: "no result", state: "no_result", normalized: "no_result", uiState: SearchTaskUIStateNoData},
		{name: "failed retryable", state: "failed_retryable", normalized: "failed_retryable", uiState: SearchTaskUIStateFailed},
		{name: "failed terminal", state: "failed_terminal", normalized: "failed_terminal", uiState: SearchTaskUIStateFailed},
		{name: "skipped", state: "skipped", normalized: "skipped", uiState: SearchTaskUIStateSkipped},
		{name: "cancelled", state: "cancelled", normalized: "cancelled", uiState: SearchTaskUIStateCancelled},
		{name: "missing", state: "missing", normalized: "missing", uiState: SearchTaskUIStateMissing},
		{name: "empty", state: "", normalized: "missing", uiState: SearchTaskUIStateMissing},
		{name: "unknown", state: "paused", normalized: "paused", uiState: SearchTaskUIStateFailed},
		{name: "trim lower", state: " Running ", normalized: "running", uiState: SearchTaskUIStateSearching},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeBOMSearchTaskState(tt.state); got != tt.normalized {
				t.Fatalf("NormalizeBOMSearchTaskState(%q)=%q want %q", tt.state, got, tt.normalized)
			}
			if got := MapBOMSearchTaskUIState(tt.state); got != tt.uiState {
				t.Fatalf("MapBOMSearchTaskUIState(%q)=%q want %q", tt.state, got, tt.uiState)
			}
		})
	}
}

func TestCanRetryBOMSearchTask(t *testing.T) {
	tests := []struct {
		name      string
		state     string
		mode      SearchTaskRetryMode
		retryable bool
	}{
		{name: "batch failed retryable", state: "failed_retryable", mode: SearchTaskRetryBatchAnomaly, retryable: true},
		{name: "batch failed terminal", state: "failed_terminal", mode: SearchTaskRetryBatchAnomaly, retryable: true},
		{name: "batch missing", state: "missing", mode: SearchTaskRetryBatchAnomaly, retryable: true},
		{name: "batch empty missing", state: "", mode: SearchTaskRetryBatchAnomaly, retryable: true},
		{name: "batch no result excluded", state: "no_result", mode: SearchTaskRetryBatchAnomaly, retryable: false},
		{name: "manual no result included", state: "no_result", mode: SearchTaskRetrySingleManual, retryable: true},
		{name: "manual failed terminal", state: "failed_terminal", mode: SearchTaskRetrySingleManual, retryable: true},
		{name: "running blocked", state: "running", mode: SearchTaskRetrySingleManual, retryable: false},
		{name: "pending blocked", state: "retry_backoff", mode: SearchTaskRetrySingleManual, retryable: false},
		{name: "succeeded blocked", state: "succeeded", mode: SearchTaskRetrySingleManual, retryable: false},
		{name: "skipped blocked", state: "skipped", mode: SearchTaskRetrySingleManual, retryable: false},
		{name: "cancelled blocked", state: "cancelled", mode: SearchTaskRetrySingleManual, retryable: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryable, reason := CanRetryBOMSearchTask(tt.state, tt.mode)
			if retryable != tt.retryable {
				t.Fatalf("retryable=%v want %v reason=%q", retryable, tt.retryable, reason)
			}
			if retryable && reason != "" {
				t.Fatalf("retryable reason=%q want empty", reason)
			}
			if !retryable && reason == "" {
				t.Fatal("blocked retry should return a Chinese reason")
			}
		})
	}
}

func TestBuildSearchTaskStatusSummary(t *testing.T) {
	rows := []SearchTaskStatusRow{
		{SearchTaskState: "pending", Retryable: false},
		{SearchTaskState: "retry_backoff", Retryable: true},
		{SearchTaskState: "running"},
		{SearchTaskState: "succeeded"},
		{SearchTaskState: "no_result", Retryable: true},
		{SearchTaskState: "failed_terminal", Retryable: true},
		{SearchTaskState: "skipped"},
		{SearchTaskState: "cancelled"},
		{SearchTaskState: ""},
		{SearchTaskState: "paused"},
		{SearchUIState: SearchTaskUIStateNoData},
	}

	got := BuildSearchTaskStatusSummary(rows)
	want := SearchTaskStatusSummary{
		Total:     11,
		Pending:   2,
		Searching: 1,
		Succeeded: 1,
		NoData:    2,
		Failed:    2,
		Skipped:   1,
		Cancelled: 1,
		Missing:   1,
		Retryable: 3,
	}

	if got != want {
		t.Fatalf("summary=%+v want %+v", got, want)
	}
}
