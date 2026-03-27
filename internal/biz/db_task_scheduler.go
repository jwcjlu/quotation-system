package biz

import (
	"context"
	"errors"
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
	_ = s.dispatch.EnqueuePending(context.Background(), t)
}

func (s *dbTaskScheduler) PullTasksForAgent(agentID string, running []RunningTaskReport, max int) []TaskMessage {
	ctx := context.Background()
	now := time.Now()
	off := BootstrapAgentOfflineThreshold(s.bc)
	offlineBefore := now.Add(-off)
	_, _ = s.dispatch.ReclaimStaleLeases(ctx, now, offlineBefore)

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
	err := s.dispatch.FinishLeased(ctx, in.TaskID, in.LeaseID, in.Status)
	if errors.Is(err, ErrDispatchLeaseMismatch) {
		return ErrLeaseReassigned
	}
	return err
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

var _ TaskScheduler = (*dbTaskScheduler)(nil)
