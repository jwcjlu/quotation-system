package service

import (
	"context"
	"strings"
	"time"

	v1 "caichip/api/admin/v1"
	"caichip/internal/biz"
	"caichip/internal/conf"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// AgentAdminService 实现 api/admin/v1.AgentAdminServiceHTTPServer（与 Agent 采集 API 分离）。
type AgentAdminService struct {
	bc       *conf.Bootstrap
	registry biz.AgentRegistryRepo
	dispatch biz.DispatchTaskRepo
	log      *log.Helper
}

// NewAgentAdminService ...
func NewAgentAdminService(
	bc *conf.Bootstrap,
	reg biz.AgentRegistryRepo,
	disp biz.DispatchTaskRepo,
	logger log.Logger,
) *AgentAdminService {
	return &AgentAdminService{bc: bc, registry: reg, dispatch: disp, log: log.NewHelper(logger)}
}

// Enabled 是否配置了 agent_admin.api_keys（用于注册 HTTP）。
func (s *AgentAdminService) Enabled() bool {
	if s == nil || s.bc == nil || s.bc.AgentAdmin == nil {
		return false
	}
	for _, k := range s.bc.AgentAdmin.ApiKeys {
		if strings.TrimSpace(k) != "" {
			return true
		}
	}
	return false
}

func (s *AgentAdminService) validateAdminKey(ctx context.Context) bool {
	if s == nil || s.bc == nil || s.bc.AgentAdmin == nil {
		return false
	}
	keys := make(map[string]struct{})
	for _, k := range s.bc.AgentAdmin.ApiKeys {
		k = strings.TrimSpace(k)
		if k != "" {
			keys[k] = struct{}{}
		}
	}
	if len(keys) == 0 {
		return false
	}
	r, ok := khttp.RequestFromServerContext(ctx)
	if !ok {
		return false
	}
	if x := strings.TrimSpace(r.Header.Get("X-API-Key")); x != "" {
		_, hit := keys[x]
		return hit
	}
	const p = "Bearer "
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, p) {
		token := strings.TrimSpace(auth[len(p):])
		_, hit := keys[token]
		return hit
	}
	return false
}

func (s *AgentAdminService) dbOK() bool {
	return s.registry != nil && s.registry.DBOk() && s.dispatch != nil && s.dispatch.DBOk()
}

// ListAgents 列出 Agent 及在线状态（与 BootstrapAgentOfflineThreshold 一致）。
func (s *AgentAdminService) ListAgents(ctx context.Context, _ *v1.ListAgentsRequest) (*v1.ListAgentsReply, error) {
	if !s.validateAdminKey(ctx) {
		return nil, kerrors.Unauthorized("UNAUTHORIZED", "invalid or missing agent admin api key")
	}
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	rows, err := s.registry.ListAgentRegistrySummaries(ctx)
	if err != nil {
		return nil, err
	}
	th := biz.BootstrapAgentOfflineThreshold(s.bc)
	now := time.Now()
	out := &v1.ListAgentsReply{Agents: make([]*v1.AgentSummary, 0, len(rows))}
	for _, r := range rows {
		online := false
		var ts *timestamppb.Timestamp
		if r.LastTaskHeartbeatAt != nil {
			ts = timestamppb.New(*r.LastTaskHeartbeatAt)
			if now.Sub(*r.LastTaskHeartbeatAt) <= th {
				online = true
			}
		}
		out.Agents = append(out.Agents, &v1.AgentSummary{
			AgentId:             r.AgentID,
			Queue:               r.Queue,
			Hostname:            r.Hostname,
			LastTaskHeartbeatAt: ts,
			Online:              online,
		})
	}
	return out, nil
}

// ListAgentLeasedTasks 某 Agent 当前租约中的调度任务。
func (s *AgentAdminService) ListAgentLeasedTasks(ctx context.Context, req *v1.ListAgentLeasedTasksRequest) (*v1.ListAgentLeasedTasksReply, error) {
	if !s.validateAdminKey(ctx) {
		return nil, kerrors.Unauthorized("UNAUTHORIZED", "invalid or missing agent admin api key")
	}
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, kerrors.BadRequest("BAD_REQUEST", "agent_id required")
	}
	rows, err := s.dispatch.ListLeasedTasksByAgent(ctx, aid)
	if err != nil {
		return nil, err
	}
	out := &v1.ListAgentLeasedTasksReply{Tasks: make([]*v1.LeasedTaskRow, 0, len(rows))}
	for _, r := range rows {
		row := &v1.LeasedTaskRow{
			TaskId:   r.TaskID,
			ScriptId: r.ScriptID,
			Version:  r.Version,
		}
		if r.LeasedAt != nil {
			row.LeasedAt = timestamppb.New(*r.LeasedAt)
		}
		if r.LeaseDeadlineAt != nil {
			row.LeaseDeadlineAt = timestamppb.New(*r.LeaseDeadlineAt)
		}
		out.Tasks = append(out.Tasks, row)
	}
	return out, nil
}

// ListAgentInstalledScripts 某 Agent 已安装脚本（来自心跳上报快照）。
func (s *AgentAdminService) ListAgentInstalledScripts(ctx context.Context, req *v1.ListAgentInstalledScriptsRequest) (*v1.ListAgentInstalledScriptsReply, error) {
	if !s.validateAdminKey(ctx) {
		return nil, kerrors.Unauthorized("UNAUTHORIZED", "invalid or missing agent admin api key")
	}
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	aid := strings.TrimSpace(req.GetAgentId())
	if aid == "" {
		return nil, kerrors.BadRequest("BAD_REQUEST", "agent_id required")
	}
	rows, err := s.registry.ListInstalledScriptsForAgent(ctx, aid)
	if err != nil {
		return nil, err
	}
	out := &v1.ListAgentInstalledScriptsReply{Scripts: make([]*v1.InstalledScriptRow, 0, len(rows))}
	for _, r := range rows {
		out.Scripts = append(out.Scripts, &v1.InstalledScriptRow{
			ScriptId:  r.ScriptID,
			Version:   r.Version,
			EnvStatus: r.EnvStatus,
			UpdatedAt: timestamppb.New(r.UpdatedAt),
		})
	}
	return out, nil
}

var _ v1.AgentAdminServiceHTTPServer = (*AgentAdminService)(nil)
