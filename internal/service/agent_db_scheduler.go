package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"caichip/internal/biz"
	"caichip/internal/conf"
	"caichip/internal/data"

	"github.com/google/uuid"
)

// dbTaskScheduler MySQL 队列表调度；心跳元数据写库由 AgentService 调 AgentRegistryRepo。
type dbTaskScheduler struct {
	hub      *biz.AgentHub
	dispatch *data.DispatchTaskRepo
	registry *data.AgentRegistryRepo
	bc       *conf.Bootstrap
}

func newDBTaskScheduler(hub *biz.AgentHub, dispatch *data.DispatchTaskRepo, reg *data.AgentRegistryRepo, bc *conf.Bootstrap) biz.TaskScheduler {
	return &dbTaskScheduler{hub: hub, dispatch: dispatch, registry: reg, bc: bc}
}

func useMySQLDispatch(bc *conf.Bootstrap) bool {
	if bc == nil || bc.Agent == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(bc.Agent.DispatchStore), "mysql")
}

func mysqlDispatchReady(bc *conf.Bootstrap, dispatch *data.DispatchTaskRepo, reg *data.AgentRegistryRepo) bool {
	return useMySQLDispatch(bc) && dispatch != nil && dispatch.DBOk() && reg != nil && reg.DBOk()
}

func newTaskScheduler(hub *biz.AgentHub, dispatch *data.DispatchTaskRepo, reg *data.AgentRegistryRepo, bc *conf.Bootstrap) biz.TaskScheduler {
	if mysqlDispatchReady(bc, dispatch, reg) {
		return newDBTaskScheduler(hub, dispatch, reg, bc)
	}
	return hub
}

func (s *dbTaskScheduler) EnqueueTask(t *biz.QueuedTask) {
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

func (s *dbTaskScheduler) PullTasksForAgent(agentID string, running []biz.RunningTaskReport, max int) []biz.TaskMessage {
	ctx := context.Background()
	now := time.Now()
	off := biz.BootstrapAgentOfflineThreshold(s.bc)
	offlineBefore := now.Add(-off)
	_, _ = s.dispatch.ReclaimStaleLeases(ctx, now, offlineBefore)

	meta, err := s.registry.LoadSchedulingMeta(ctx, agentID)
	if err != nil || meta == nil {
		meta = &biz.AgentSchedulingMeta{
			Queue:   "default",
			Tags:    make(map[string]struct{}),
			Scripts: make(map[string]biz.InstalledScript),
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

func (s *dbTaskScheduler) SubmitTaskResult(in *biz.TaskResultIn) error {
	ctx := context.Background()
	err := s.dispatch.FinishLeased(ctx, in.TaskID, in.LeaseID, in.Status)
	if errors.Is(err, data.ErrDispatchLeaseMismatch) {
		return biz.ErrLeaseReassigned
	}
	return err
}

func (s *dbTaskScheduler) TouchTaskHeartbeat(agentID string) {
	s.hub.TouchTaskHeartbeat(agentID)
}

func (s *dbTaskScheduler) UpdateAgentMeta(agentID, queue string, tags []string, scripts []biz.InstalledScript) {
	s.hub.UpdateAgentMeta(agentID, queue, tags, scripts)
}

func (s *dbTaskScheduler) WaitForLongPoll(ctx context.Context, agentID string, running []biz.RunningTaskReport, maxWait, pollEvery time.Duration) []biz.TaskMessage {
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
