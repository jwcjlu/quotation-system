package biz

import (
	"context"
	"testing"
	"time"

	"caichip/internal/conf"
)

func testHub() *AgentHub {
	return NewAgentHub(&conf.Bootstrap{
		Agent: &conf.Agent{
			DefaultTaskHeartbeatSec:    10,
			OfflineMinSec:              120,
			OfflineHeartbeatMultiplier: 6,
		},
	})
}

func TestAgentHub_offlineReassign(t *testing.T) {
	h := testHub()
	agent := "aabbccddeeff"
	h.TouchTaskHeartbeat(agent)
	h.UpdateAgentMeta(agent, "default", nil, []InstalledScript{
		{ScriptID: "demo", Version: "1.0.0", EnvStatus: "ready"},
	})
	h.EnqueueTask(&QueuedTask{
		TaskMessage: TaskMessage{TaskID: "t1", ScriptID: "demo", Version: "1.0.0", Attempt: 1},
		Queue:       "default",
	})
	tasks := h.PullTasksForAgent(agent, 4)
	if len(tasks) != 1 || tasks[0].LeaseID == "" {
		t.Fatalf("expected 1 task with lease, got %+v", tasks)
	}
	lease := tasks[0].LeaseID
	// 模拟离线：很久未心跳
	h.mu.Lock()
	h.lastTaskHB[agent] = time.Now().Add(-200 * time.Second)
	h.mu.Unlock()
	h.TouchTaskHeartbeat("other") // 触发 reassignStaleLocked 在别的心跳里
	// 再次 Touch 当前 agent 不应自动收回任务；应已重入队
	h.TouchTaskHeartbeat(agent)
	h.UpdateAgentMeta(agent, "default", nil, []InstalledScript{
		{ScriptID: "demo", Version: "1.0.0", EnvStatus: "ready"},
	})
	tasks2 := h.PullTasksForAgent(agent, 4)
	if len(tasks2) != 1 {
		t.Fatalf("expected task re-queued after offline, got %d", len(tasks2))
	}
	if tasks2[0].LeaseID == lease {
		t.Fatal("lease should change after reassign")
	}
}

func TestAgentHub_leaseMismatch(t *testing.T) {
	h := testHub()
	agent := "aabbccddeeff"
	h.TouchTaskHeartbeat(agent)
	h.UpdateAgentMeta(agent, "default", nil, []InstalledScript{
		{ScriptID: "demo", Version: "v1.0.0", EnvStatus: "ready"},
	})
	h.EnqueueTask(&QueuedTask{
		TaskMessage: TaskMessage{TaskID: "t2", ScriptID: "demo", Version: "1.0.0", Attempt: 1},
		Queue:       "default",
	})
	tasks := h.PullTasksForAgent(agent, 4)
	err := h.SubmitTaskResult(&TaskResultIn{
		TaskID:  "t2",
		AgentID: agent,
		LeaseID: "wrong-lease",
		Status:  "success",
	})
	if err != ErrLeaseReassigned {
		t.Fatalf("expected ErrLeaseReassigned, got %v", err)
	}
	_ = tasks
}

func TestAgentHub_WaitForLongPoll(t *testing.T) {
	h := testHub()
	agent := "aabbccddeeff"
	h.TouchTaskHeartbeat(agent)
	h.UpdateAgentMeta(agent, "default", nil, []InstalledScript{
		{ScriptID: "demo", Version: "1.0.0", EnvStatus: "ready"},
	})
	go func() {
		time.Sleep(50 * time.Millisecond)
		h.EnqueueTask(&QueuedTask{
			TaskMessage: TaskMessage{TaskID: "t3", ScriptID: "demo", Version: "1.0.0", Attempt: 1},
			Queue:       "default",
		})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out := h.WaitForLongPoll(ctx, agent, 500*time.Millisecond, 30*time.Millisecond)
	if len(out) != 1 {
		t.Fatalf("expected 1 task, got %d", len(out))
	}
}
