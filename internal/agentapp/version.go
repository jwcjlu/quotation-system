package agentapp

import "strings"

// Normalize 与需求 §6.5 一致：去空白、去掉可选 v/V 前缀。
func Normalize(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == 'v' || s[0] == 'V') {
		return s[1:]
	}
	return s
}

// Equal 规范化后相等。
func Equal(a, b string) bool {
	return Normalize(a) == Normalize(b)
}
