package biz

import "strings"

// ReadinessMode 会话级就绪策略（与设计 §2.3 一致）。
const (
	ReadinessLenient = "lenient"
	ReadinessStrict  = "strict"
)

// LineReadinessSnapshot 一行在就绪判定中的最小信息（以 mpn_norm 对齐搜索任务键）。
type LineReadinessSnapshot struct {
	MpnNorm string
}

// TaskReadinessSnapshot 任务状态快照（同会话内键为 mpn_norm + platform_id）。
type TaskReadinessSnapshot struct {
	MpnNorm    string
	PlatformID string
	State      string
}

// platformTerminalStates 完成类状态：不阻塞「搜索阶段完成」（设计 §2.1 / §3.4）。
var platformTerminalStates = map[string]struct{}{
	"succeeded":       {},
	"no_result":       {},
	"failed_terminal": {},
	"cancelled":       {},
	"skipped":         {},
}

func isPlatformTerminal(state string) bool {
	s := strings.ToLower(strings.TrimSpace(state))
	_, ok := platformTerminalStates[s]
	return ok
}

// ReadinessFromTasks 纯函数：在给定行 × 勾选平台 下是否满足「可标数据已准备」的搜索侧条件（不含写库）。
// mode 为 ReadinessLenient 或 ReadinessStrict（其它值按 lenient 处理）。
func ReadinessFromTasks(mode string, tasks []TaskReadinessSnapshot, lines []LineReadinessSnapshot, platformIDs []string) bool {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if len(lines) == 0 || len(platformIDs) == 0 {
		return true
	}

	taskState := make(map[string]string)
	for _, t := range tasks {
		k := taskKey(strings.TrimSpace(t.MpnNorm), strings.TrimSpace(t.PlatformID))
		if k == "\x00" {
			continue
		}
		taskState[k] = strings.ToLower(strings.TrimSpace(t.State))
	}

	for _, line := range lines {
		mn := strings.TrimSpace(line.MpnNorm)
		if mn == "" {
			continue
		}
		for _, pid := range platformIDs {
			pid = strings.TrimSpace(pid)
			if pid == "" {
				continue
			}
			k := taskKey(mn, pid)
			st, ok := taskState[k]
			if !ok || !isPlatformTerminal(st) {
				return false
			}
		}
	}

	if mode == ReadinessStrict {
		for _, line := range lines {
			mn := strings.TrimSpace(line.MpnNorm)
			if mn == "" {
				continue
			}
			anySucceeded := false
			for _, pid := range platformIDs {
				pid = strings.TrimSpace(pid)
				if pid == "" {
					continue
				}
				k := taskKey(mn, pid)
				if taskState[k] == "succeeded" {
					anySucceeded = true
					break
				}
			}
			if !anySucceeded {
				return false
			}
		}
	}

	return true
}

func taskKey(mpnNorm, platformID string) string {
	return mpnNorm + "\x00" + platformID
}
