package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	v1 "caichip/api/admin/v1"
	"caichip/internal/biz"
	"caichip/internal/conf"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// AgentAdminService 实现 api/admin/v1.AgentAdminServiceHTTPServer（与 Agent 采集 API 分离）。
type AgentAdminService struct {
	bc         *conf.Bootstrap
	registry   biz.AgentRegistryRepo
	dispatch   biz.DispatchTaskRepo
	scriptAuth biz.AgentScriptAuthRepo
	platforms  biz.BomPlatformScriptRepo
	log        *log.Helper
}

// NewAgentAdminService ...
func NewAgentAdminService(
	bc *conf.Bootstrap,
	reg biz.AgentRegistryRepo,
	disp biz.DispatchTaskRepo,
	scriptAuth biz.AgentScriptAuthRepo,
	platforms biz.BomPlatformScriptRepo,
	logger log.Logger,
) *AgentAdminService {
	return &AgentAdminService{
		bc: bc, registry: reg, dispatch: disp, scriptAuth: scriptAuth, platforms: platforms, log: log.NewHelper(logger),
	}
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
	return s.registry != nil && s.registry.DBOk() && s.dispatch != nil && s.dispatch.DBOk() &&
		s.scriptAuth != nil && s.scriptAuth.DBOk() && s.platforms != nil && s.platforms.DBOk()
}

func bomPlatformRowToProto(p *biz.BomPlatformScript) (*v1.BomPlatformRow, error) {
	if p == nil {
		return nil, nil
	}
	var st *structpb.Struct
	if len(p.RunParamsJSON) > 0 {
		var m map[string]interface{}
		if err := json.Unmarshal(p.RunParamsJSON, &m); err != nil {
			return nil, err
		}
		var err error
		st, err = structpb.NewStruct(m)
		if err != nil {
			return nil, err
		}
	}
	return &v1.BomPlatformRow{
		PlatformId:  p.PlatformID,
		ScriptId:    p.ScriptID,
		DisplayName: p.DisplayName,
		Enabled:     p.Enabled,
		RunParams:   st,
		UpdatedAt:   timestamppb.New(p.UpdatedAt),
	}, nil
}

// ListAgents 列出 Agent 及在线状态（与 BootstrapAgentOfflineThreshold 一致）。
func (s *AgentAdminService) ListAgents(ctx context.Context, _ *v1.ListAgentsRequest) (*v1.ListAgentsReply, error) {
	if !s.validateAdminKey(ctx) {
		return nil, kerrors.Unauthorized("UNAUTHORIZED", "invalid or missing agent admin api key")
	}
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	// BootstrapAgentOfflineThreshold 返回 time.Duration（配置里的秒已乘 time.Second）
	th := biz.BootstrapAgentOfflineThreshold(s.bc)
	now := time.Now()
	cutoff := now.Add(-th)
	if _, err := s.registry.MarkAgentsOfflineBefore(ctx, cutoff); err != nil {
		return nil, err
	}
	rows, err := s.registry.ListAgentRegistrySummaries(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.ListAgentsReply{
		Agents:           make([]*v1.AgentSummary, 0, len(rows)),
		OfflineWindowSec: int32(th / time.Second),
	}
	for _, r := range rows {
		online := false
		statusStr := biz.AgentStatusUnknown
		var ts *timestamppb.Timestamp
		if r.LastTaskHeartbeatAt != nil {
			ts = timestamppb.New(*r.LastTaskHeartbeatAt)
			if now.Sub(*r.LastTaskHeartbeatAt) <= th {
				online = true
				statusStr = biz.AgentStatusOnline
			} else {
				statusStr = biz.AgentStatusOffline
			}
		}
		out.Agents = append(out.Agents, &v1.AgentSummary{
			AgentId:             r.AgentID,
			Queue:               r.Queue,
			Hostname:            r.Hostname,
			LastTaskHeartbeatAt: ts,
			Online:              online,
			Status:              statusStr,
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

// ListAgentScriptAuths 列出某 Agent 的 script 凭据（不含密码）。
func (s *AgentAdminService) ListAgentScriptAuths(ctx context.Context, req *v1.ListAgentScriptAuthsRequest) (*v1.ListAgentScriptAuthsReply, error) {
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
	rows, err := s.scriptAuth.ListByAgent(ctx, aid)
	if err != nil {
		return nil, err
	}
	out := &v1.ListAgentScriptAuthsReply{Rows: make([]*v1.AgentScriptAuthRow, 0, len(rows))}
	for _, r := range rows {
		out.Rows = append(out.Rows, &v1.AgentScriptAuthRow{
			ScriptId:  r.ScriptID,
			Username:  r.Username,
			UpdatedAt: timestamppb.New(r.UpdatedAt),
		})
	}
	return out, nil
}

// UpsertAgentScriptAuth 写入或更新凭据（须配置 AES 密钥）。
func (s *AgentAdminService) UpsertAgentScriptAuth(ctx context.Context, req *v1.UpsertAgentScriptAuthRequest) (*v1.UpsertAgentScriptAuthReply, error) {
	if !s.validateAdminKey(ctx) {
		return nil, kerrors.Unauthorized("UNAUTHORIZED", "invalid or missing agent admin api key")
	}
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	if s.scriptAuth == nil || !s.scriptAuth.CipherConfigured() {
		return nil, kerrors.BadRequest("SCRIPT_AUTH_CIPHER_DISABLED", "configure CAICHIP_AGENT_SCRIPT_AUTH_KEY or agent_script_auth.aes_key_base64 (32-byte key base64)")
	}
	aid := strings.TrimSpace(req.GetAgentId())
	sid := strings.TrimSpace(req.GetScriptId())
	if err := s.scriptAuth.Upsert(ctx, aid, sid, req.GetUsername(), req.GetPassword()); err != nil {
		return nil, kerrors.BadRequest("SCRIPT_AUTH_UPSERT", err.Error())
	}
	return &v1.UpsertAgentScriptAuthReply{}, nil
}

// DeleteAgentScriptAuth 删除一行凭据。
func (s *AgentAdminService) DeleteAgentScriptAuth(ctx context.Context, req *v1.DeleteAgentScriptAuthRequest) (*v1.DeleteAgentScriptAuthReply, error) {
	if !s.validateAdminKey(ctx) {
		return nil, kerrors.Unauthorized("UNAUTHORIZED", "invalid or missing agent admin api key")
	}
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	aid := strings.TrimSpace(req.GetAgentId())
	sid := strings.TrimSpace(req.GetScriptId())
	if err := s.scriptAuth.Delete(ctx, aid, sid); err != nil {
		return nil, err
	}
	return &v1.DeleteAgentScriptAuthReply{}, nil
}

// ListBomPlatforms 列出 BOM 采集平台及 run_params。
func (s *AgentAdminService) ListBomPlatforms(ctx context.Context, _ *v1.ListBomPlatformsRequest) (*v1.ListBomPlatformsReply, error) {
	if !s.validateAdminKey(ctx) {
		return nil, kerrors.Unauthorized("UNAUTHORIZED", "invalid or missing agent admin api key")
	}
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	rows, err := s.platforms.List(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.ListBomPlatformsReply{Items: make([]*v1.BomPlatformRow, 0, len(rows))}
	for i := range rows {
		row, err := bomPlatformRowToProto(&rows[i])
		if err != nil {
			return nil, kerrors.InternalServer("BOM_PLATFORM_SERIALIZE", err.Error())
		}
		out.Items = append(out.Items, row)
	}
	return out, nil
}

// GetBomPlatform 按 platform_id 查询一行。
func (s *AgentAdminService) GetBomPlatform(ctx context.Context, req *v1.GetBomPlatformRequest) (*v1.GetBomPlatformReply, error) {
	if !s.validateAdminKey(ctx) {
		return nil, kerrors.Unauthorized("UNAUTHORIZED", "invalid or missing agent admin api key")
	}
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	pid := strings.TrimSpace(req.GetPlatformId())
	if pid == "" {
		return nil, kerrors.BadRequest("BAD_REQUEST", "platform_id required")
	}
	row, err := s.platforms.Get(ctx, pid)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, kerrors.NotFound("BOM_PLATFORM_NOT_FOUND", "platform not found")
	}
	item, err := bomPlatformRowToProto(row)
	if err != nil {
		return nil, kerrors.InternalServer("BOM_PLATFORM_SERIALIZE", err.Error())
	}
	return &v1.GetBomPlatformReply{Item: item}, nil
}

// UpsertBomPlatform 创建或全量更新；要求 platform_id 与 script_id 一致。
func (s *AgentAdminService) UpsertBomPlatform(ctx context.Context, req *v1.UpsertBomPlatformRequest) (*v1.UpsertBomPlatformReply, error) {
	if !s.validateAdminKey(ctx) {
		return nil, kerrors.Unauthorized("UNAUTHORIZED", "invalid or missing agent admin api key")
	}
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	pid := strings.TrimSpace(req.GetPlatformId())
	sid := strings.TrimSpace(req.GetScriptId())
	if pid == "" || sid == "" {
		return nil, kerrors.BadRequest("BAD_REQUEST", "platform_id and script_id required")
	}
	if pid != sid {
		return nil, kerrors.BadRequest("BOM_PLATFORM_ID_MISMATCH", "platform_id must equal script_id")
	}
	var raw []byte
	if req.GetRunParams() != nil {
		m := req.GetRunParams().AsMap()
		b, err := json.Marshal(m)
		if err != nil {
			return nil, kerrors.BadRequest("RUN_PARAMS_JSON", err.Error())
		}
		raw = b
	}
	if _, err := biz.ExpandRunParamsJSON(raw); err != nil {
		return nil, kerrors.BadRequest("RUN_PARAMS_INVALID", err.Error())
	}
	row := &biz.BomPlatformScript{
		PlatformID:    pid,
		ScriptID:      sid,
		DisplayName:   strings.TrimSpace(req.GetDisplayName()),
		Enabled:       req.GetEnabled(),
		RunParamsJSON: raw,
	}
	if err := s.platforms.Upsert(ctx, row); err != nil {
		return nil, err
	}
	saved, err := s.platforms.Get(ctx, pid)
	if err != nil {
		return nil, err
	}
	if saved == nil {
		return nil, kerrors.InternalServer("BOM_PLATFORM_UPSERT", "row missing after upsert")
	}
	item, err := bomPlatformRowToProto(saved)
	if err != nil {
		return nil, kerrors.InternalServer("BOM_PLATFORM_SERIALIZE", err.Error())
	}
	return &v1.UpsertBomPlatformReply{Item: item}, nil
}

// DeleteBomPlatform 物理删除一行（停用请用 Upsert enabled=false）。
func (s *AgentAdminService) DeleteBomPlatform(ctx context.Context, req *v1.DeleteBomPlatformRequest) (*v1.DeleteBomPlatformReply, error) {
	if !s.validateAdminKey(ctx) {
		return nil, kerrors.Unauthorized("UNAUTHORIZED", "invalid or missing agent admin api key")
	}
	if !s.dbOK() {
		return nil, kerrors.ServiceUnavailable("DB_DISABLED", "database not configured")
	}
	pid := strings.TrimSpace(req.GetPlatformId())
	if pid == "" {
		return nil, kerrors.BadRequest("BAD_REQUEST", "platform_id required")
	}
	if err := s.platforms.Delete(ctx, pid); err != nil {
		return nil, err
	}
	return &v1.DeleteBomPlatformReply{}, nil
}

var _ v1.AgentAdminServiceHTTPServer = (*AgentAdminService)(nil)
