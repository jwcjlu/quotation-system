package biz

import (
	"strconv"
	"strings"
)

// parseQtyText 从左到右提取首段数字，支持 "10000-12000"/"10000pcs" 等格式。
func parseQtyText(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return strconv.ParseFloat(s, 64)
	}
	end := 0
	dotSeen := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			end = i + 1
			continue
		}
		if ch == '.' && !dotSeen {
			dotSeen = true
			end = i + 1
			continue
		}
		break
	}
	if end > 0 {
		s = s[:end]
	}
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}
