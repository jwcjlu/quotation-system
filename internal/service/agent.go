package service

import (
	"context"
	"errors"
	"strings"
	"time"

	v1 "caichip/api/agent/v1"
	"caichip/internal/biz"
	"caichip/internal/conf"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
)

// AgentService Agent HTTP API 业务层（实现 api/agent/v1.AgentServiceHTTPServer）。
type AgentService struct {
	hub         *biz.AgentHub
	log         *log.Helper
	keys        map[string]struct{}
	longPollMax int
	enabled     bool
	devEnqueue  bool
}

// NewAgentService 创建 Agent 服务；未配置 agent 或 enabled=false 时路由可不注册。
func NewAgentService(hub *biz.AgentHub, bc *conf.Bootstrap, logger log.Logger) *AgentService {
	keys := make(map[string]struct{})
	enabled := false
	devEnq := false
	maxSec := 55
	if bc != nil && bc.Agent != nil {
		a := bc.Agent
		enabled = a.Enabled
		devEnq = a.DevEnqueueEnabled
		for _, k := range a.ApiKeys {
			if k != "" {
				keys[k] = struct{}{}
			}
		}
		if a.LongPollMaxSec > 0 {
			maxSec = a.LongPollMaxSec
		}
	}
	return &AgentService{
		hub:         hub,
		log:         log.NewHelper(logger),
		keys:        keys,
		longPollMax: maxSec,
		enabled:     enabled,
		devEnqueue:  devEnq,
	}
}

// Enabled 是否启用 Agent API。
func (s *AgentService) Enabled() bool { return s.enabled }

// DevEnqueueEnabled 是否开放开发入队接口。
func (s *AgentService) DevEnqueueEnabled() bool { return s.devEnqueue }

// DevEnqueue 开发入队（仅配置 dev_enqueue_enabled 时可用）。
func (s *AgentService) DevEnqueue(t *biz.QueuedTask) {
	s.hub.EnqueueTask(t)
}

// ValidateAPIKey 校验 Header 中的 API Key。
func (s *AgentService) ValidateAPIKey(authBearer, xAPIKey string) bool {
	if len(s.keys) == 0 {
		return false
	}
	if xAPIKey != "" {
		_, ok := s.keys[xAPIKey]
		return ok
	}
	const p = "Bearer "
	if strings.HasPrefix(authBearer, p) {
		token := strings.TrimSpace(authBearer[len(p):])
		_, ok := s.keys[token]
		return ok
	}
	return false
}

// TaskHeartbeat 任务心跳 + 长轮询拉任务。
func (s *AgentService) TaskHeartbeat(ctx context.Context, req *v1.TaskHeartbeatRequest) (*v1.TaskHeartbeatReply, error) {
	if req.GetAgentId() == "" {
		return nil, errBadRequest("agent_id required")
	}
	s.hub.TouchTaskHeartbeat(req.GetAgentId())
	scripts := make([]biz.InstalledScript, len(req.GetInstalledScripts()))
	for i, x := range req.GetInstalledScripts() {
		if x == nil {
			continue
		}
		scripts[i] = biz.InstalledScript{
			ScriptID:  x.GetScriptId(),
			Version:   x.GetVersion(),
			EnvStatus: x.GetEnvStatus(),
		}
	}
	s.hub.UpdateAgentMeta(req.GetAgentId(), req.GetQueue(), req.GetTags(), scripts)

	lp := int(req.GetLongPollTimeoutSec())
	if lp <= 0 {
		lp = 50
	}
	if lp > s.longPollMax {
		lp = s.longPollMax
	}
	wait := time.Duration(lp) * time.Second
	tasks := s.hub.WaitForLongPoll(ctx, req.GetAgentId(), wait, 150*time.Millisecond)

	out := &v1.TaskHeartbeatReply{
		ServerTime:         time.Now().UTC().Format(time.RFC3339Nano),
		LongPollTimeoutSec: int32(lp),
		Tasks:              make([]*v1.TaskObject, 0, len(tasks)),
	}
	for _, t := range tasks {
		to := &v1.TaskObject{
			TaskId:     t.TaskID,
			ScriptId:   t.ScriptID,
			Version:    t.Version,
			EntryFile:  t.EntryFile,
			TimeoutSec: int32(t.TimeoutSec),
			LeaseId:    t.LeaseID,
		}
		if to.TimeoutSec == 0 {
			to.TimeoutSec = 300
		}
		out.Tasks = append(out.Tasks, to)
	}
	return out, nil
}

// ScriptSyncHeartbeat 脚本安装心跳（当前返回无同步动作，占位）。
func (s *AgentService) ScriptSyncHeartbeat(_ context.Context, req *v1.ScriptSyncHeartbeatRequest) (*v1.ScriptSyncHeartbeatReply, error) {
	if req.GetAgentId() == "" {
		return nil, errBadRequest("agent_id required")
	}
	scripts := make([]biz.InstalledScript, len(req.GetScripts()))
	for i, x := range req.GetScripts() {
		if x == nil {
			continue
		}
		scripts[i] = biz.InstalledScript{
			ScriptID:  x.GetScriptId(),
			Version:   x.GetVersion(),
			EnvStatus: x.GetEnvStatus(),
		}
	}
	s.hub.TouchTaskHeartbeat(req.GetAgentId())
	s.hub.UpdateAgentMeta(req.GetAgentId(), req.GetQueue(), req.GetTags(), scripts)

	lp := int(req.GetLongPollTimeoutSec())
	if lp <= 0 {
		lp = 50
	}
	if lp > s.longPollMax {
		lp = s.longPollMax
	}
	return &v1.ScriptSyncHeartbeatReply{
		ServerTime:         time.Now().UTC().Format(time.RFC3339Nano),
		LongPollTimeoutSec: int32(lp),
		SyncActions:        nil,
	}, nil
}

// TaskResult 任务结果上报。
func (s *AgentService) TaskResult(_ context.Context, req *v1.TaskResultRequest) (*v1.TaskResultReply, error) {
	if req.GetTaskId() == "" || req.GetAgentId() == "" {
		return nil, errBadRequest("task_id and agent_id required")
	}
	attempt := int(req.GetAttempt())
	if attempt == 0 {
		attempt = 1
	}
	in := &biz.TaskResultIn{
		TaskID:  req.GetTaskId(),
		AgentID: req.GetAgentId(),
		LeaseID: req.GetLeaseId(),
		Status:  req.GetStatus(),
		Attempt: attempt,
	}
	if err := s.hub.SubmitTaskResult(in); err != nil {
		if errors.Is(err, biz.ErrLeaseReassigned) {
			return nil, kerrors.Conflict("LEASE_EXPIRED", "task reassigned or lease invalid")
		}
		return nil, err
	}
	return &v1.TaskResultReply{
		Accepted:   true,
		ServerTime: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

// BadRequestError 400 业务错误。
type BadRequestError struct {
	Message string
}

func (e *BadRequestError) Error() string { return e.Message }

func errBadRequest(msg string) error { return kerrors.BadRequest("BAD_REQUEST", msg) }
