package biz

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// NormalizeMfrString 厂牌字符串规范化：trim、NFKC（全角拉丁等 → 半角兼容形）、再对 Unicode 字母做大小写折叠。
// 与 NormalizeMPNForBOMSearch 一致之处：均使用 strings.ToUpper 做字母大小写统一；厂牌路径额外做 NFKC 以满足设计 §2 全半角约定
// （MPN 搜索键当前仅 trim + ToUpper，二者在仅含半角字母数字时行为一致）。
func NormalizeMfrString(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = norm.NFKC.String(s)
	return strings.ToUpper(s)
}

