package service

import (
	"context"
	"testing"
	"time"

	v1 "caichip/api/agent/v1"
	"caichip/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
)

type stubTaskScheduler struct {
	lastResult *biz.TaskResultIn
}

func (s *stubTaskScheduler) EnqueueTask(t *biz.QueuedTask) {}

func (s *stubTaskScheduler) PullTasksForAgent(agentID string, running []biz.RunningTaskReport, max int) []biz.TaskMessage {
	return nil
}

func (s *stubTaskScheduler) SubmitTaskResult(in *biz.TaskResultIn) error {
	s.lastResult = in
	return nil
}

func (s *stubTaskScheduler) TouchTaskHeartbeat(agentID string) {}

func (s *stubTaskScheduler) UpdateAgentMeta(agentID, queue string, tags []string, scripts []biz.InstalledScript) {
}

func (s *stubTaskScheduler) WaitForLongPoll(ctx context.Context, agentID string, running []biz.RunningTaskReport, maxWait, pollEvery time.Duration) []biz.TaskMessage {
	return nil
}

func TestAgentService_TaskResultPassesErrorMessage(t *testing.T) {
	sched := &stubTaskScheduler{}
	svc := NewAgentService(nil, sched, nil, nil, nil, nil, nil, testAgentConf(), log.DefaultLogger)

	_, err := svc.TaskResult(context.Background(), &v1.TaskResultRequest{
		AgentId:      "agent-1",
		TaskId:       "task-1",
		LeaseId:      "lease-1",
		Status:       "failed",
		ErrorMessage: "site login failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sched.lastResult == nil || sched.lastResult.ErrorMessage != "site login failed" {
		t.Fatalf("unexpected result payload: %+v", sched.lastResult)
	}
}

func TestAgentService_TaskResultAcceptsLegacyStdoutTail(t *testing.T) {
	sched := &stubTaskScheduler{}
	svc := NewAgentService(nil, sched, nil, nil, nil, nil, nil, testAgentConf(), log.DefaultLogger)

	_, err := svc.TaskResult(context.Background(), &v1.TaskResultRequest{
		AgentId:    "agent-1",
		TaskId:     "task-1",
		LeaseId:    "lease-1",
		Status:     "success",
		StdoutTail: `[{"model":"ABC"}]`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sched.lastResult == nil || sched.lastResult.Stdout != `[{"model":"ABC"}]` {
		t.Fatalf("unexpected result payload: %+v", sched.lastResult)
	}
}
