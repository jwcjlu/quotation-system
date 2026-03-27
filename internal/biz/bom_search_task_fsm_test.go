package biz

import "testing"

func TestFSM_PendingClaimDispatch(t *testing.T) {
	to, err := BomSearchTaskTransition("pending", "claim_dispatch")
	if err != nil || to != "running" {
		t.Fatalf("got %q %v", to, err)
	}
}

func TestFSM_RunningErrorRetryable(t *testing.T) {
	to, err := BomSearchTaskTransition("running", "error_retryable")
	if err != nil || to != "failed_retryable" {
		t.Fatalf("got %q %v", to, err)
	}
}

func TestFSM_RetryExhaustedToTerminal(t *testing.T) {
	to, err := BomSearchTaskTransition("failed_retryable", "attempts_exhausted")
	if err != nil || to != "failed_terminal" {
		t.Fatalf("got %q %v", to, err)
	}
}

func TestFSM_Invalid(t *testing.T) {
	_, err := BomSearchTaskTransition("pending", "result_ok_with_quotes")
	if err != ErrInvalidTaskTransition {
		t.Fatalf("expected ErrInvalidTaskTransition, got %v", err)
	}
}

func TestFSM_CaseInsensitive(t *testing.T) {
	to, err := BomSearchTaskTransition("PENDING", "Claim_Dispatch")
	if err != nil || to != "running" {
		t.Fatalf("got %q %v", to, err)
	}
}
