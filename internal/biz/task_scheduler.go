package biz

import (
	"context"
	"time"
)

// TaskScheduler Agent 任务调度（内存 Hub 或 MySQL 队列表实现）。
type TaskScheduler interface {
	EnqueueTask(t *QueuedTask)
	PullTasksForAgent(agentID string, running []RunningTaskReport, max int) []TaskMessage
	SubmitTaskResult(in *TaskResultIn) error
	TouchTaskHeartbeat(agentID string)
	UpdateAgentMeta(agentID, queue string, tags []string, scripts []InstalledScript)
	WaitForLongPoll(ctx context.Context, agentID string, running []RunningTaskReport, maxWait, pollEvery time.Duration) []TaskMessage
}

var _ TaskScheduler = (*AgentHub)(nil)
