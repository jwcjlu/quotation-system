package data

import (
	"context"
	"strings"
	"time"

	"caichip/internal/biz"
)

// CachedAgentRegistryRepo 仅缓存 ListInstalledScriptsForAgent；心跳写库后删键。
type CachedAgentRegistryRepo struct {
	inner *AgentRegistryRepo
	kv    *InprocKV
}

// NewCachedAgentRegistryRepo ...
func NewCachedAgentRegistryRepo(inner *AgentRegistryRepo, kv *InprocKV) *CachedAgentRegistryRepo {
	if inner == nil {
		inner = &AgentRegistryRepo{}
	}
	if kv == nil {
		kv = NewInprocKV()
	}
	return &CachedAgentRegistryRepo{inner: inner, kv: kv}
}

func cloneInstalledScriptRows(in []biz.AgentInstalledScriptRow) []biz.AgentInstalledScriptRow {
	out := make([]biz.AgentInstalledScriptRow, len(in))
	copy(out, in)
	return out
}

func (r *CachedAgentRegistryRepo) DBOk() bool {
	return r.inner.DBOk()
}

func (r *CachedAgentRegistryRepo) UpsertTaskHeartbeat(ctx context.Context, agentID, queue, hostname string, scripts []biz.InstalledScript, tags []string) error {
	if err := r.inner.UpsertTaskHeartbeat(ctx, agentID, queue, hostname, scripts, tags); err != nil {
		return err
	}
	aid := strings.TrimSpace(agentID)
	if aid != "" {
		r.kv.Delete(KeyAgInstAgent(aid))
	}
	return nil
}

func (r *CachedAgentRegistryRepo) MarkAgentsOfflineBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	return r.inner.MarkAgentsOfflineBefore(ctx, cutoff)
}

func (r *CachedAgentRegistryRepo) LoadSchedulingMeta(ctx context.Context, agentID string) (*biz.AgentSchedulingMeta, error) {
	return r.inner.LoadSchedulingMeta(ctx, agentID)
}

func (r *CachedAgentRegistryRepo) ListAgentRegistrySummaries(ctx context.Context) ([]biz.AgentRegistrySummary, error) {
	return r.inner.ListAgentRegistrySummaries(ctx)
}

func (r *CachedAgentRegistryRepo) ListInstalledScriptsForAgent(ctx context.Context, agentID string) ([]biz.AgentInstalledScriptRow, error) {
	key := KeyAgInstAgent(strings.TrimSpace(agentID))
	if v, ok := r.kv.Get(key); ok {
		if rows, ok := v.([]biz.AgentInstalledScriptRow); ok {
			return cloneInstalledScriptRows(rows), nil
		}
	}
	rows, err := r.inner.ListInstalledScriptsForAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	r.kv.Set(key, cloneInstalledScriptRows(rows))
	return cloneInstalledScriptRows(rows), nil
}

var _ biz.AgentRegistryRepo = (*CachedAgentRegistryRepo)(nil)
