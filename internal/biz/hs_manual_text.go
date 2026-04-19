package biz

import "strings"

// SanitizeManualComponentDescription 与 datasheet 抽取侧类似的可见字符白名单，用于 run 指纹与 prompt 前清洗。
func SanitizeManualComponentDescription(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || (r >= 32 && r <= 126) || (r >= 0x4e00 && r <= 0x9fff) {
			b.WriteRune(r)
			continue
		}
		b.WriteRune(' ')
	}
	return strings.TrimSpace(b.String())
}
