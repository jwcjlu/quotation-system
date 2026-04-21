package biz

import (
	"context"
	"errors"
	"sync"
	"time"

	"caichip/internal/conf"

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
	TaskID     string                 `json:"task_id"`
	ScriptID   string                 `json:"script_id"`
	Version    string                 `json:"version"`
	EntryFile  *string                `json:"entry_file,omitempty"`
	Argv       []string               `json:"argv,omitempty"`
	Params     map[string]interface{} `json:"params,omitempty"`
	TimeoutSec int                    `json:"timeout_sec,omitempty"`
	LeaseID    string                 `json:"lease_id,omitempty"`
	Attempt    int                    `json:"attempt,omitempty"`
}

// QueuedTask 调度队列中的任务。
type QueuedTask struct {
	TaskMessage
	Queue           string   `json:"queue"`
	RequiredTags    []string `json:"-"` // 可选：非空则 Agent 须包含这些 tag
	DefaultQueue    bool     // 若 true 且 Queue 空则匹配 default
	RetryMax        *int     `json:"-"`
	RetryBackoffSec []int    `json:"-"`
	// NextClaimAt 若设置，则在到达该时间前任务不可被 Agent 认领（t_caichip_dispatch_task.next_claim_at）。
	NextClaimAt *time.Time `json:"-"`
}

// TaskResultIn 结果上报入参。
type TaskResultIn struct {
	TaskID  string `json:"task_id"`
	AgentID string `json:"agent_id"`
	LeaseID string `json:"lease_id"`
	Status  string `json:"status"`
	Attempt int    `json:"attempt"`
	// Stdout Agent 采集脚本标准输出（各平台统一报价 JSON）；与 proto TaskResultRequest.stdout 对齐。
	Stdout       string `json:"stdout,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
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

// RunningTaskReport 任务心跳上报的「本机正在执行」快照；与调度侧重复派发防护（方案 A）对齐。
type RunningTaskReport struct {
	TaskID    string
	LeaseID   string
	ScriptID  string
	StartedAt string
}

func runningOccupiedSets(running []RunningTaskReport) (taskIDs, scriptIDs map[string]struct{}) {
	taskIDs = make(map[string]struct{})
	scriptIDs = make(map[string]struct{})
	for _, r := range running {
		if r.TaskID != "" {
			taskIDs[r.TaskID] = struct{}{}
		}
		if r.ScriptID != "" {
			scriptIDs[r.ScriptID] = struct{}{}
		}
	}
	return taskIDs, scriptIDs
}

// AgentHub 内存态 Agent 注册、任务派发、租约与重派（生产可换 Redis/DB）。
type AgentHub struct {
	mu sync.RWMutex
	c  *conf.Bootstrap

	lastTaskHB map[string]time.Time           // agent_id -> 最后一次成功任务心跳时间
	meta       map[string]AgentSchedulingMeta // agent_id
	pending    []*QueuedTask
	assign     map[string]*assignment // task_id
}

// NewAgentHub 创建 Hub；c.Agent 可为 nil（使用默认参数）。
func NewAgentHub(c *conf.Bootstrap) *AgentHub {
	return &AgentHub{
		c:          c,
		lastTaskHB: make(map[string]time.Time),
		meta:       make(map[string]AgentSchedulingMeta),
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
	return BootstrapAgentOfflineThreshold(h.c)
}

// BootstrapAgentOfflineThreshold 与 AgentHub.reassignStaleLocked 使用的离线窗口一致。
func BootstrapAgentOfflineThreshold(c *conf.Bootstrap) time.Duration {
	ac := &conf.Agent{}
	if c != nil && c.Agent != nil {
		ac = c.Agent
	}
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
	m := AgentSchedulingMeta{Queue: queue, Tags: make(map[string]struct{}), Scripts: make(map[string]InstalledScript)}
	for _, t := range tags {
		m.Tags[t] = struct{}{}
	}
	for _, s := range scripts {
		m.Scripts[s.ScriptID] = s
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
// running：本心跳上报的「正在执行」集合；非空时用于跳过已占用的 task_id 及同一 script_id 的并行派发（串行语义）。
func (h *AgentHub) PullTasksForAgent(agentID string, running []RunningTaskReport, max int) []TaskMessage {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reassignStaleLocked()
	if max <= 0 {
		max = 8
	}
	schedMeta := h.meta[agentID]
	busyTaskIDs, busyScriptIDs := runningOccupiedSets(running)
	var out []TaskMessage
	var rest []*QueuedTask
	for _, t := range h.pending {
		if len(out) >= max {
			rest = append(rest, t)
			continue
		}
		if _, ok := busyTaskIDs[t.TaskID]; ok {
			rest = append(rest, t)
			continue
		}
		if _, ok := busyScriptIDs[t.ScriptID]; ok {
			rest = append(rest, t)
			continue
		}
		if !MatchTaskForAgent(&schedMeta, t) {
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
func (h *AgentHub) WaitForLongPoll(ctx context.Context, agentID string, running []RunningTaskReport, maxWait time.Duration, pollEvery time.Duration) []TaskMessage {
	deadline := time.Now().Add(maxWait)
	if pollEvery <= 0 {
		pollEvery = 200 * time.Millisecond
	}
	for {
		tasks := h.PullTasksForAgent(agentID, running, 4)
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
