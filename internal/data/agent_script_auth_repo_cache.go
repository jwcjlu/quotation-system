package data

import (
	"context"
	"strings"

	"caichip/internal/biz"
)

// CachedAgentScriptAuthRepo Agent 凭据读穿 + 密文单行缓存 + 列表缓存。
type CachedAgentScriptAuthRepo struct {
	inner *AgentScriptAuthRepo
	kv    *InprocKV
}

// NewCachedAgentScriptAuthRepo ...
func NewCachedAgentScriptAuthRepo(inner *AgentScriptAuthRepo, kv *InprocKV) *CachedAgentScriptAuthRepo {
	if inner == nil {
		inner = &AgentScriptAuthRepo{}
	}
	if kv == nil {
		kv = NewInprocKV()
	}
	return &CachedAgentScriptAuthRepo{inner: inner, kv: kv}
}

func cloneAgentScriptSummaries(in []biz.AgentScriptAuthSummary) []biz.AgentScriptAuthSummary {
	out := make([]biz.AgentScriptAuthSummary, len(in))
	copy(out, in)
	return out
}

type scriptAuthMaterialCache struct {
	Username       string
	PasswordCipher string
}

func (r *CachedAgentScriptAuthRepo) DBOk() bool {
	return r.inner.DBOk()
}

func (r *CachedAgentScriptAuthRepo) CipherConfigured() bool {
	return r.inner.CipherConfigured()
}

func (r *CachedAgentScriptAuthRepo) GetPlatformAuth(ctx context.Context, agentID, scriptID string) (username, password string, ok bool, err error) {
	if !r.inner.DBOk() {
		return r.inner.GetPlatformAuth(ctx, agentID, scriptID)
	}
	if !r.inner.CipherConfigured() {
		return r.inner.GetPlatformAuth(ctx, agentID, scriptID)
	}
	aid := strings.TrimSpace(agentID)
	sid := strings.TrimSpace(scriptID)
	if aid == "" || sid == "" {
		return "", "", false, nil
	}
	pairKey := KeyAsAuthPair(aid, sid)
	if v, hit := r.kv.Get(pairKey); hit {
		if m, ok := v.(*scriptAuthMaterialCache); ok {
			pw, derr := r.inner.decryptStoredPassword(m.PasswordCipher)
			if derr != nil || pw == "" {
				r.kv.Delete(pairKey)
				return r.inner.GetPlatformAuth(ctx, agentID, scriptID)
			}
			return m.Username, pw, true, nil
		}
	}
	u, ct, ok, err := r.inner.GetPlatformAuthMaterial(ctx, agentID, scriptID)
	if err != nil || !ok {
		return u, "", ok, err
	}
	r.kv.Set(pairKey, &scriptAuthMaterialCache{Username: u, PasswordCipher: ct})
	pw, derr := r.inner.decryptStoredPassword(ct)
	if derr != nil {
		return "", "", false, nil
	}
	return u, pw, true, nil
}

func (r *CachedAgentScriptAuthRepo) ListByAgent(ctx context.Context, agentID string) ([]biz.AgentScriptAuthSummary, error) {
	key := KeyAsAuthAgent(strings.TrimSpace(agentID))
	if v, ok := r.kv.Get(key); ok {
		if rows, ok := v.([]biz.AgentScriptAuthSummary); ok {
			return cloneAgentScriptSummaries(rows), nil
		}
	}
	rows, err := r.inner.ListByAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	r.kv.Set(key, cloneAgentScriptSummaries(rows))
	return cloneAgentScriptSummaries(rows), nil
}

func (r *CachedAgentScriptAuthRepo) Upsert(ctx context.Context, agentID, scriptID, username, password string) error {
	if err := r.inner.Upsert(ctx, agentID, scriptID, username, password); err != nil {
		return err
	}
	r.invalidateAgentScriptKeys(agentID, scriptID)
	return nil
}

func (r *CachedAgentScriptAuthRepo) Delete(ctx context.Context, agentID, scriptID string) error {
	if err := r.inner.Delete(ctx, agentID, scriptID); err != nil {
		return err
	}
	r.invalidateAgentScriptKeys(agentID, scriptID)
	return nil
}

func (r *CachedAgentScriptAuthRepo) invalidateAgentScriptKeys(agentID, scriptID string) {
	aid := strings.TrimSpace(agentID)
	sid := strings.TrimSpace(scriptID)
	if aid != "" {
		r.kv.Delete(KeyAsAuthAgent(aid))
	}
	if aid != "" && sid != "" {
		r.kv.Delete(KeyAsAuthPair(aid, sid))
	}
}

var _ biz.AgentScriptAuthRepo = (*CachedAgentScriptAuthRepo)(nil)
