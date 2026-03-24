package service

import (
	"context"
	"testing"

	v1 "caichip/api/agent/v1"
	"caichip/internal/biz"
	"caichip/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
)

func testAgentConf() *conf.Bootstrap {
	return &conf.Bootstrap{
		Agent: &conf.Agent{
			Enabled:                    true,
			ApiKeys:                    []string{"test-api-key"},
			LongPollMaxSec:             55,
			DefaultTaskHeartbeatSec:    10,
			OfflineMinSec:              120,
			OfflineHeartbeatMultiplier: 6,
		},
	}
}

func TestAgentService_Auth(t *testing.T) {
	s := NewAgentService(biz.NewAgentHub(testAgentConf()), nil, nil, nil, nil, testAgentConf(), log.DefaultLogger)
	if s.ValidateAPIKey("", "") {
		t.Fatal("empty should fail")
	}
	if !s.ValidateAPIKey("Bearer test-api-key", "") {
		t.Fatal("bearer should pass")
	}
	if !s.ValidateAPIKey("", "test-api-key") {
		t.Fatal("x-api-key should pass")
	}
}

func TestAgentService_TaskHeartbeatPull(t *testing.T) {
	bc := testAgentConf()
	h := biz.NewAgentHub(bc)
	s := NewAgentService(h, nil, nil, nil, nil, bc, log.DefaultLogger)
	h.EnqueueTask(&biz.QueuedTask{
		TaskMessage: biz.TaskMessage{
			TaskID:   "tid-1",
			ScriptID: "demo",
			Version:  "1.0.0",
			Attempt:  1,
		},
		Queue: "default",
	})
	req := &v1.TaskHeartbeatRequest{
		AgentId: "agent-1",
		Queue:   "default",
		InstalledScripts: []*v1.InstalledScript{
			{ScriptId: "demo", Version: "v1.0.0", EnvStatus: "ready"},
		},
		LongPollTimeoutSec: 2,
	}
	resp, err := s.TaskHeartbeat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Tasks) != 1 || resp.Tasks[0].GetLeaseId() == "" {
		t.Fatalf("tasks: %+v", resp.Tasks)
	}
}
