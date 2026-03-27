package biz

import (
	"strings"

	"caichip/internal/conf"
)

// UseMySQLDispatch 是否使用 MySQL 队列表调度。
func UseMySQLDispatch(bc *conf.Bootstrap) bool {
	if bc == nil || bc.Agent == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(bc.Agent.DispatchStore), "mysql")
}

// MySQLDispatchReady MySQL 调度与 Agent 注册表均已就绪。
func MySQLDispatchReady(bc *conf.Bootstrap, dispatch DispatchTaskRepo, reg AgentRegistryRepo) bool {
	return UseMySQLDispatch(bc) && dispatch != nil && dispatch.DBOk() && reg != nil && reg.DBOk()
}

// NewAgentTaskScheduler 内存 Hub 或 MySQL 调度二选一。
func NewAgentTaskScheduler(hub *AgentHub, dispatch DispatchTaskRepo, reg AgentRegistryRepo, bc *conf.Bootstrap) TaskScheduler {
	if MySQLDispatchReady(bc, dispatch, reg) {
		return newDBTaskScheduler(hub, dispatch, reg, bc)
	}
	return hub
}
