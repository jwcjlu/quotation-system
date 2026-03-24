package biz

import "strings"

// NormalizeMPNForTask 与 bom_search_task.mpn_norm / EnsureTasksForSession 规则一致。
func NormalizeMPNForTask(mpn string) string {
	m := strings.TrimSpace(mpn)
	if m == "" {
		return "-"
	}
	return strings.ToUpper(m)
}
