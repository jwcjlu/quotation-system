package biz

import (
	"context"
	"testing"
	"time"

	"caichip/internal/conf"
)

type dispatchRetryTestRepo struct {
	loaded    *DispatchLeasedTask
	loadErr   error
	stale     []StaleDispatchTask
	staleErr  error
	finished  *dispatchFinishCall
	retries   []dispatchRetryCall
	terminals []dispatchTerminalCall
}

type dispatchFinishCall struct {
	taskID       string
	leaseID      string
	resultStatus string
}

type dispatchRetryCall struct {
	taskID      string
	leaseID     string
	nextAttempt int
	nextClaimAt time.Time
	lastError   string
}

type dispatchTerminalCall struct {
	taskID       string
	leaseID      string
	resultStatus string
	lastError    string
	finishedAt   time.Time
}

func (r *dispatchRetryTestRepo) DBOk() bool                                              { return true }
func (r *dispatchRetryTestRepo) Ping(ctx context.Context) error                          { return nil }
func (r *dispatchRetryTestRepo) EnqueuePending(ctx context.Context, t *QueuedTask) error { return nil }
func (r *dispatchRetryTestRepo) ReclaimStaleLeases(ctx context.Context, now, offlineBefore time.Time) (int64, error) {
	return 0, nil
}
func (r *dispatchRetryTestRepo) PullAndLeaseForAgent(ctx context.Context, queue, agentID string, meta *AgentSchedulingMeta, running []RunningTaskReport, max int, leaseExtraSec int32) ([]TaskMessage, error) {
	return nil, nil
}
func (r *dispatchRetryTestRepo) ListLeasedTasksByAgent(ctx context.Context, agentID string) ([]LeasedDispatchTaskRow, error) {
	return nil, nil
}
func (r *dispatchRetryTestRepo) LoadLeasedTask(ctx context.Context, taskID, leaseID string) (*DispatchLeasedTask, error) {
	return r.loaded, r.loadErr
}
func (r *dispatchRetryTestRepo) ListStaleLeasedTasks(ctx context.Context, now, offlineBefore time.Time) ([]StaleDispatchTask, error) {
	return r.stale, r.staleErr
}
func (r *dispatchRetryTestRepo) FinishLeased(ctx context.Context, taskID, leaseID, resultStatus string) error {
	r.finished = &dispatchFinishCall{taskID: taskID, leaseID: leaseID, resultStatus: resultStatus}
	return nil
}
func (r *dispatchRetryTestRepo) RequeueLeased(ctx context.Context, taskID, leaseID string, nextAttempt int, nextClaimAt time.Time, lastError string) error {
	r.retries = append(r.retries, dispatchRetryCall{
		taskID:      taskID,
		leaseID:     leaseID,
		nextAttempt: nextAttempt,
		nextClaimAt: nextClaimAt,
		lastError:   lastError,
	})
	return nil
}
func (r *dispatchRetryTestRepo) FailLeasedTerminal(ctx context.Context, taskID, leaseID, resultStatus, lastError string, finishedAt time.Time) error {
	r.terminals = append(r.terminals, dispatchTerminalCall{
		taskID:       taskID,
		leaseID:      leaseID,
		resultStatus: resultStatus,
		lastError:    lastError,
		finishedAt:   finishedAt,
	})
	return nil
}

func TestDBTaskScheduler_SubmitTaskResultFailedSchedulesRetry(t *testing.T) {
	repo := &dispatchRetryTestRepo{
		loaded: &DispatchLeasedTask{
			TaskID:          "task-1",
			LeaseID:         "lease-1",
			Attempt:         1,
			RetryMax:        3,
			RetryBackoffSec: []int{60, 300, 900},
		},
	}
	s := newDBTaskScheduler(nil, repo, nil, &conf.Bootstrap{})
	before := time.Now()

	if err := s.SubmitTaskResult(&TaskResultIn{
		TaskID:       "task-1",
		LeaseID:      "lease-1",
		Status:       "failed",
		ErrorMessage: "proxy rejected",
	}); err != nil {
		t.Fatal(err)
	}

	if repo.finished != nil {
		t.Fatalf("did not expect finish call, got %+v", repo.finished)
	}
	if len(repo.retries) != 1 {
		t.Fatalf("expected one retry call, got %+v", repo.retries)
	}
	call := repo.retries[0]
	if call.nextAttempt != 2 || call.lastError != "proxy rejected" {
		t.Fatalf("unexpected retry call: %+v", call)
	}
	if delta := call.nextClaimAt.Sub(before); delta < 55*time.Second || delta > 65*time.Second {
		t.Fatalf("expected retry around 60s, got %v", delta)
	}
}

func TestDBTaskScheduler_SubmitTaskResultFailedExhaustsToTerminal(t *testing.T) {
	repo := &dispatchRetryTestRepo{
		loaded: &DispatchLeasedTask{
			TaskID:          "task-1",
			LeaseID:         "lease-1",
			Attempt:         4,
			RetryMax:        3,
			RetryBackoffSec: []int{60, 300, 900},
		},
	}
	s := newDBTaskScheduler(nil, repo, nil, &conf.Bootstrap{})

	if err := s.SubmitTaskResult(&TaskResultIn{
		TaskID:       "task-1",
		LeaseID:      "lease-1",
		Status:       "failed",
		ErrorMessage: "captcha loop",
	}); err != nil {
		t.Fatal(err)
	}

	if len(repo.retries) != 0 {
		t.Fatalf("did not expect retry calls, got %+v", repo.retries)
	}
	if len(repo.terminals) != 1 {
		t.Fatalf("expected one terminal call, got %+v", repo.terminals)
	}
	call := repo.terminals[0]
	if call.resultStatus != dispatchResultFailedTerminal || call.lastError != "captcha loop" {
		t.Fatalf("unexpected terminal call: %+v", call)
	}
}

func TestDBTaskScheduler_ReclaimStaleLeasesUsesRetryBudget(t *testing.T) {
	now := time.Now()
	repo := &dispatchRetryTestRepo{
		stale: []StaleDispatchTask{
			{
				DispatchLeasedTask: DispatchLeasedTask{
					TaskID:          "task-retry",
					LeaseID:         "lease-retry",
					Attempt:         1,
					RetryMax:        3,
					RetryBackoffSec: []int{60, 300, 900},
				},
				FailureReason: DispatchFailureReasonLeaseExpired,
			},
			{
				DispatchLeasedTask: DispatchLeasedTask{
					TaskID:          "task-terminal",
					LeaseID:         "lease-terminal",
					Attempt:         4,
					RetryMax:        3,
					RetryBackoffSec: []int{60, 300, 900},
				},
				FailureReason: DispatchFailureReasonAgentOfflineReclaimed,
			},
		},
	}
	s := newDBTaskScheduler(nil, repo, nil, &conf.Bootstrap{})

	s.reclaimStaleLeases(context.Background(), now, now.Add(-time.Minute))

	if len(repo.retries) != 1 || repo.retries[0].taskID != "task-retry" {
		t.Fatalf("unexpected retry reclaim calls: %+v", repo.retries)
	}
	if len(repo.terminals) != 1 || repo.terminals[0].taskID != "task-terminal" {
		t.Fatalf("unexpected terminal reclaim calls: %+v", repo.terminals)
	}
}

func TestDBTaskScheduler_SubmitTaskResultMapsLeaseMismatch(t *testing.T) {
	repo := &dispatchRetryTestRepo{loadErr: ErrDispatchLeaseMismatch}
	s := newDBTaskScheduler(nil, repo, nil, &conf.Bootstrap{})

	err := s.SubmitTaskResult(&TaskResultIn{TaskID: "task-1", LeaseID: "lease-1", Status: "failed"})
	if err != ErrLeaseReassigned {
		t.Fatalf("expected ErrLeaseReassigned, got %v", err)
	}
}
