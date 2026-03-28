package biz

import (
	"context"
	"time"
)

// AgentScriptAuthSummary 运维列表一行（不含密码）。
type AgentScriptAuthSummary struct {
	ScriptID  string
	Username  string
	UpdatedAt time.Time
}

// AgentScriptAuthRepo Agent×script_id 登录凭据（实现位于 internal/data）。
type AgentScriptAuthRepo interface {
	DBOk() bool
	CipherConfigured() bool
	GetPlatformAuth(ctx context.Context, agentID, scriptID string) (username, password string, ok bool, err error)
	ListByAgent(ctx context.Context, agentID string) ([]AgentScriptAuthSummary, error)
	Upsert(ctx context.Context, agentID, scriptID, username, password string) error
	Delete(ctx context.Context, agentID, scriptID string) error
}
