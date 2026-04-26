package biz

import (
	"strings"
	"time"
)

const (
	dispatchResultFailedTerminal               = "failed_terminal"
	DispatchFailureReasonLeaseExpired          = "LEASE_EXPIRED"
	DispatchFailureReasonAgentOfflineReclaimed = "AGENT_OFFLINE_RECLAIMED"
	dispatchFailureReasonFallback              = "task failed"
	dispatchFailureReasonMaxLen                = 512
)

type DispatchFailureTransition struct {
	NextAttempt  int
	RetryAt      *time.Time
	ResultStatus string
	LastError    string
	FinishedAt   time.Time
}

func DispatchFailureTransitionForTask(now time.Time, base DispatchRetryPolicy, task DispatchLeasedTask, reason string) DispatchFailureTransition {
	policy := base
	if task.RetryMax >= 0 {
		policy.RetryMax = task.RetryMax
	}
	if xs := sanitizeDispatchRetryBackoffInts(task.RetryBackoffSec); len(xs) > 0 {
		policy.BackoffSec = xs
	}
	out := DispatchFailureTransition{
		ResultStatus: dispatchResultFailedTerminal,
		LastError:    normalizeDispatchFailureReason(reason),
		FinishedAt:   now,
	}
	if delay, ok := policy.DelayForFailedAttempt(task.Attempt); ok {
		next := now.Add(delay)
		out.NextAttempt = task.Attempt + 1
		out.RetryAt = &next
	}
	return out
}

func DispatchFailureReasonFromResult(in *TaskResultIn) string {
	if in == nil {
		return dispatchFailureReasonFallback
	}
	if s := strings.TrimSpace(in.ErrorMessage); s != "" {
		return normalizeDispatchFailureReason(s)
	}
	if s := strings.TrimSpace(in.Stdout); s != "" {
		return normalizeDispatchFailureReason(s)
	}
	return dispatchFailureReasonFallback
}

func normalizeDispatchFailureReason(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return dispatchFailureReasonFallback
	}
	if len(s) > dispatchFailureReasonMaxLen {
		return s[:dispatchFailureReasonMaxLen]
	}
	return s
}
