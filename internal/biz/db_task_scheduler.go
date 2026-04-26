package biz

import (
	"context"
	"errors"
	"strings"
	"time"

	"caichip/internal/conf"

	"github.com/google/uuid"
)

type dbTaskScheduler struct {
	hub      *AgentHub
	dispatch DispatchTaskRepo
	registry AgentRegistryRepo
	bc       *conf.Bootstrap
}

func newDBTaskScheduler(hub *AgentHub, dispatch DispatchTaskRepo, reg AgentRegistryRepo, bc *conf.Bootstrap) *dbTaskScheduler {
	return &dbTaskScheduler{hub: hub, dispatch: dispatch, registry: reg, bc: bc}
}

func (s *dbTaskScheduler) EnqueueTask(t *QueuedTask) {
	if t == nil {
		return
	}
	if t.TaskID == "" {
		t.TaskID = uuid.NewString()
	}
	if t.Attempt <= 0 {
		t.Attempt = 1
	}
	if t.Queue == "" {
		t.Queue = "default"
	}
	policy := DispatchRetryPolicyFromBootstrap(s.bc)
	if t.RetryMax == nil {
		retryMax := policy.RetryMax
		t.RetryMax = &retryMax
	}
	if len(t.RetryBackoffSec) == 0 {
		t.RetryBackoffSec = append([]int(nil), policy.BackoffSec...)
	}
	_ = s.dispatch.EnqueuePending(context.Background(), t)
}

func (s *dbTaskScheduler) PullTasksForAgent(agentID string, running []RunningTaskReport, max int) []TaskMessage {
	ctx := context.Background()
	now := time.Now()
	off := BootstrapAgentOfflineThreshold(s.bc)
	offlineBefore := now.Add(-off)
	s.reclaimStaleLeases(ctx, now, offlineBefore)

	meta, err := s.registry.LoadSchedulingMeta(ctx, agentID)
	if err != nil || meta == nil {
		meta = &AgentSchedulingMeta{
			Queue:   "default",
			Tags:    make(map[string]struct{}),
			Scripts: make(map[string]InstalledScript),
		}
	}
	queue := meta.Queue
	if queue == "" {
		queue = "default"
	}
	var extra int32
	if s.bc != nil && s.bc.Agent != nil {
		extra = s.bc.Agent.DispatchLeaseExtraSec
	}
	out, err := s.dispatch.PullAndLeaseForAgent(ctx, queue, agentID, meta, running, max, extra)
	if err != nil {
		return nil
	}
	return out
}

func (s *dbTaskScheduler) SubmitTaskResult(in *TaskResultIn) error {
	ctx := context.Background()
	if strings.EqualFold(strings.TrimSpace(in.Status), "success") {
		return s.mapLeaseErr(s.dispatch.FinishLeased(ctx, in.TaskID, in.LeaseID, in.Status))
	}
	task, err := s.dispatch.LoadLeasedTask(ctx, in.TaskID, in.LeaseID)
	if err != nil || task == nil {
		return s.mapLeaseErr(err)
	}
	transition := DispatchFailureTransitionForTask(time.Now(), DispatchRetryPolicyFromBootstrap(s.bc), *task, DispatchFailureReasonFromResult(in))
	if transition.RetryAt != nil {
		return s.mapLeaseErr(s.dispatch.RequeueLeased(ctx, task.TaskID, task.LeaseID, transition.NextAttempt, *transition.RetryAt, transition.LastError))
	}
	return s.mapLeaseErr(s.dispatch.FailLeasedTerminal(ctx, task.TaskID, task.LeaseID, transition.ResultStatus, transition.LastError, transition.FinishedAt))
}

func (s *dbTaskScheduler) TouchTaskHeartbeat(agentID string) {
	s.hub.TouchTaskHeartbeat(agentID)
}

func (s *dbTaskScheduler) UpdateAgentMeta(agentID, queue string, tags []string, scripts []InstalledScript) {
	s.hub.UpdateAgentMeta(agentID, queue, tags, scripts)
}

func (s *dbTaskScheduler) WaitForLongPoll(ctx context.Context, agentID string, running []RunningTaskReport, maxWait, pollEvery time.Duration) []TaskMessage {
	deadline := time.Now().Add(maxWait)
	if pollEvery <= 0 {
		pollEvery = 200 * time.Millisecond
	}
	for {
		tasks := s.PullTasksForAgent(agentID, running, 4)
		if len(tasks) > 0 {
			return tasks
		}
		if time.Now().After(deadline) {
			return nil
		}
		remaining := time.Until(deadline)
		if remaining > pollEvery {
			remaining = pollEvery
		}
		t := time.NewTimer(remaining)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil
		case <-t.C:
		}
	}
}

func (s *dbTaskScheduler) reclaimStaleLeases(ctx context.Context, now, offlineBefore time.Time) {
	stale, err := s.dispatch.ListStaleLeasedTasks(ctx, now, offlineBefore)
	if err != nil {
		return
	}
	base := DispatchRetryPolicyFromBootstrap(s.bc)
	for _, task := range stale {
		transition := DispatchFailureTransitionForTask(now, base, task.DispatchLeasedTask, task.FailureReason)
		if transition.RetryAt != nil {
			_ = s.mapLeaseErr(s.dispatch.RequeueLeased(ctx, task.TaskID, task.LeaseID, transition.NextAttempt, *transition.RetryAt, transition.LastError))
			continue
		}
		_ = s.mapLeaseErr(s.dispatch.FailLeasedTerminal(ctx, task.TaskID, task.LeaseID, transition.ResultStatus, transition.LastError, transition.FinishedAt))
	}
}

func (s *dbTaskScheduler) mapLeaseErr(err error) error {
	if errors.Is(err, ErrDispatchLeaseMismatch) {
		return ErrLeaseReassigned
	}
	return err
}

var _ TaskScheduler = (*dbTaskScheduler)(nil)
