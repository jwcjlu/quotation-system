package biz

import "caichip/internal/pkg/versionutil"

// AgentSchedulingMeta Agent 侧用于任务匹配的队列、标签与已安装脚本快照（内存 Hub 与 DB 调度共用）。
type AgentSchedulingMeta struct {
	Queue   string
	Tags    map[string]struct{}
	Scripts map[string]InstalledScript // script_id -> latest
}

// MatchTaskForAgent 是否与该 Agent 能力/队列匹配（与原 matchLocked 语义一致）。
func MatchTaskForAgent(meta *AgentSchedulingMeta, t *QueuedTask) bool {
	if meta == nil || t == nil {
		return false
	}
	qTask := t.Queue
	if qTask == "" {
		qTask = "default"
	}
	qAgent := meta.Queue
	if qAgent == "" {
		qAgent = "default"
	}
	if qTask != qAgent {
		return false
	}
	for _, req := range t.RequiredTags {
		if _, ok := meta.Tags[req]; !ok {
			return false
		}
	}
	s, ok := meta.Scripts[t.ScriptID]
	if !ok || s.EnvStatus != "ready" {
		return false
	}
	return versionutil.Equal(s.Version, t.Version)
}
