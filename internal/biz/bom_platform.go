package biz

import "strings"

// NormalizePlatformID 导入/会话侧平台 ID 归一化（设计 §1：find_chip → find_chips）。
func NormalizePlatformID(id string) string {
	s := strings.TrimSpace(strings.ToLower(id))
	if s == "find_chip" {
		return "find_chips"
	}
	return s
}
