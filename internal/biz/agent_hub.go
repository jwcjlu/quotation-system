package biz

import (
	"context"
	"errors"
	"sync"
	"time"

	"caichip/internal/conf"
	"caichip/internal/pkg/versionutil"

	"github.com/google/uuid"
)

// —— 与 API 协议 / 需求文档对齐的内存实现 ——

// InstalledScript 任务心跳中的脚本快照。
type InstalledScript struct {
	ScriptID  string `json:"script_id"`
	Version   string `json:"version"`
	EnvStatus string `json:"env_status"`
}

// TaskMessage 下发给 Agent 的任务（任务心跳响应 tasks[]）。
type TaskMessage struct {
	TaskID     string  `json:"task_id"`
	ScriptID   string  `json:"script_id"`
	Version    string  `json:"version"`
	EntryFile  *string `json:"entry_file,omitempty"`
	TimeoutSec int     `json:"timeout_sec,omitempty"`
	LeaseID    string  `json:"lease_id,omitempty"`
	Attempt    int     `json:"attempt,omitempty"`
}

// QueuedTask 调度队列中的任务。
type QueuedTask struct {
	TaskMessage
	Queue        string   `json:"queue"`
	RequiredTags []string `json:"-"` // 可选：非空则 Agent 须包含这些 tag
	DefaultQueue bool     // 若 true 且 Queue 空则匹配 default
}

// TaskResultIn 结果上报入参。
type TaskResultIn struct {
	TaskID  string `json:"task_id"`
	AgentID string `json:"agent_id"`
	LeaseID string `json:"lease_id"`
	Status  string `json:"status"`
	Attempt int    `json:"attempt"`
}

type assignment struct {
	task       *QueuedTask
	agentID    string
	leaseID    string
	attempt    int
	dispatched time.Time
	terminal   bool
	resultFrom string // agent_id that reported terminal
}

// AgentHub 内存态 Agent 注册、任务派发、租约与重派（生产可换 Redis/DB）。
type AgentHub struct {
	mu sync.RWMutex
	c  *conf.Bootstrap

	lastTaskHB map[string]time.Time // agent_id -> 最后一次成功任务心跳时间
	meta       map[string]agentMeta // agent_id
	pending    []*QueuedTask
	assign     map[string]*assignment // task_id
}

type agentMeta struct {
	queue   string
	tags    map[string]struct{}
	scripts map[string]InstalledScript // script_id -> latest
}

// NewAgentHub 创建 Hub；c.Agent 可为 nil（使用默认参数）。
func NewAgentHub(c *conf.Bootstrap) *AgentHub {
	return &AgentHub{
		c:          c,
		lastTaskHB: make(map[string]time.Time),
		meta:       make(map[string]agentMeta),
		assign:     make(map[string]*assignment),
	}
}

func (h *AgentHub) agentCfg() *conf.Agent {
	if h.c != nil && h.c.Agent != nil {
		return h.c.Agent
	}
	return &conf.Agent{}
}

func (h *AgentHub) offlineThresholdSec() time.Duration {
	ac := h.agentCfg()
	interval := ac.DefaultTaskHeartbeatSec
	if interval <= 0 {
		interval = 10
	}
	mult := ac.OfflineHeartbeatMultiplier
	if mult <= 0 {
		mult = 6
	}
	min := ac.OfflineMinSec
	if min <= 0 {
		min = 120
	}
	sec := max(min, mult*interval)
	return time.Duration(sec) * time.Second
}

// TouchTaskHeartbeat 更新 Agent 在线时间并尝试将已离线 Agent 上的任务重派。
func (h *AgentHub) TouchTaskHeartbeat(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastTaskHB[agentID] = time.Now()
	h.reassignStaleLocked()
}

// UpdateAgentMeta 更新队列、标签、已安装脚本（用于调度与离线判断）。
func (h *AgentHub) UpdateAgentMeta(agentID, queue string, tags []string, scripts []InstalledScript) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if queue == "" {
		queue = "default"
	}
	m := agentMeta{queue: queue, tags: make(map[string]struct{}), scripts: make(map[string]InstalledScript)}
	for _, t := range tags {
		m.tags[t] = struct{}{}
	}
	for _, s := range scripts {
		m.scripts[s.ScriptID] = s
	}
	h.meta[agentID] = m
}

func (h *AgentHub) reassignStaleLocked() {
	th := h.offlineThresholdSec()
	now := time.Now()
	offlineAgents := make(map[string]bool)
	for aid, last := range h.lastTaskHB {
		if now.Sub(last) > th {
			offlineAgents[aid] = true
		}
	}
	for tid, as := range h.assign {
		if as.terminal {
			continue
		}
		if offlineAgents[as.agentID] {
			h.returnToPendingLocked(as)
			delete(h.assign, tid)
		}
	}
}

func (h *AgentHub) returnToPendingLocked(as *assignment) {
	t := *as.task
	t.Attempt = as.attempt + 1
	h.pending = append(h.pending, &t)
}

// PullTasksForAgent 拉取匹配该 Agent 的待派发任务（生成 lease）。
func (h *AgentHub) PullTasksForAgent(agentID string, max int) []TaskMessage {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reassignStaleLocked()
	if max <= 0 {
		max = 8
	}
	agentMeta := h.meta[agentID]
	var out []TaskMessage
	var rest []*QueuedTask
	for _, t := range h.pending {
		if len(out) >= max {
			rest = append(rest, t)
			continue
		}
		if !h.matchLocked(agentID, &agentMeta, t) {
			rest = append(rest, t)
			continue
		}
		lease := uuid.NewString()
		msg := t.TaskMessage
		msg.LeaseID = lease
		if msg.Attempt == 0 {
			msg.Attempt = 1
		}
		msg.TimeoutSec = t.TimeoutSec
		out = append(out, msg)
		h.assign[t.TaskID] = &assignment{
			task:       t,
			agentID:    agentID,
			leaseID:    lease,
			attempt:    msg.Attempt,
			dispatched: time.Now(),
			terminal:   false,
		}
	}
	h.pending = rest
	return out
}

func (h *AgentHub) matchLocked(agentID string, am *agentMeta, t *QueuedTask) bool {
	qTask := t.Queue
	if qTask == "" {
		qTask = "default"
	}
	qAgent := am.queue
	if qAgent == "" {
		qAgent = "default"
	}
	if qTask != qAgent {
		return false
	}
	for _, req := range t.RequiredTags {
		if _, ok := am.tags[req]; !ok {
			return false
		}
	}
	s, ok := am.scripts[t.ScriptID]
	if !ok || s.EnvStatus != "ready" {
		return false
	}
	return versionutil.Equal(s.Version, t.Version)
}

// EnqueueTask 入队（测试或管理接口）。
func (h *AgentHub) EnqueueTask(t *QueuedTask) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if t.TaskID == "" {
		t.TaskID = uuid.NewString()
	}
	if t.Attempt == 0 {
		t.Attempt = 1
	}
	if t.Queue == "" {
		t.Queue = "default"
	}
	h.pending = append(h.pending, t)
}

// SubmitTaskResult 上报结果；错误 ErrLeaseReassigned 表示应返回 409。
var ErrLeaseReassigned = errors.New("lease invalid or task reassigned")

func (h *AgentHub) SubmitTaskResult(in *TaskResultIn) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	as, ok := h.assign[in.TaskID]
	if !ok {
		// 无派发记录：可能已终态清理，幂等接受
		return nil
	}
	if as.terminal {
		// 同一次执行重复上报：幂等
		return nil
	}
	if in.LeaseID != "" && in.LeaseID != as.leaseID {
		return ErrLeaseReassigned
	}
	as.terminal = true
	as.resultFrom = in.AgentID
	return nil
}

// IsAgentOnline 用于测试。
func (h *AgentHub) IsAgentOnline(agentID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	last, ok := h.lastTaskHB[agentID]
	if !ok {
		return false
	}
	return time.Since(last) <= h.offlineThresholdSec()
}

// WaitForLongPoll 长轮询等待直到有任务或 ctx 取消/超时。
func (h *AgentHub) WaitForLongPoll(ctx context.Context, agentID string, maxWait time.Duration, pollEvery time.Duration) []TaskMessage {
	deadline := time.Now().Add(maxWait)
	if pollEvery <= 0 {
		pollEvery = 200 * time.Millisecond
	}
	for {
		tasks := h.PullTasksForAgent(agentID, 4)
		if len(tasks) > 0 {
			return tasks
		}
		if time.Now().After(deadline) {
			return nil
		}
		remaining := time.Until(deadline)
		if remaining > pollEvery {
			remaining = pollEvery
		}
		t := time.NewTimer(remaining)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil
		case <-t.C:
		}
	}
}
