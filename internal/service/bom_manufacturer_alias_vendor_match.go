package service

import (
	"strings"
	"unicode/utf8"

	"caichip/internal/biz"
)

// manufacturerAliasDemandQuotePlausibleSameVendor 判断需求厂牌与报价厂牌是否可能为同一标准厂（用于过滤陈旧行 canonical 造成的假差异）。
// 大小写由 biz.NormalizeMfrString 统一；含「括号前品牌键」前缀等价（如 ESPRESSIF(乐鑫) vs Espressif Systems）。
func manufacturerAliasDemandQuotePlausibleSameVendor(demandMfr, quoteMfr string) bool {
	d := biz.NormalizeMfrString(strings.TrimSpace(demandMfr))
	q := biz.NormalizeMfrString(strings.TrimSpace(quoteMfr))
	if d == "" || q == "" {
		return false
	}
	if d == q {
		return true
	}
	kd := mfrLeadingBrandKeyFromNorm(d)
	kq := mfrLeadingBrandKeyFromNorm(q)
	if kd != "" && kd == kq && utf8.RuneCountInString(kd) >= 4 {
		return true
	}
	fd := firstMfrTokenNorm(d)
	fq := firstMfrTokenNorm(q)
	if len(fd) < 3 || len(fq) < 3 || fd != fq {
		return false
	}
	if utf8.RuneCountInString(fd) > 6 {
		return false
	}
	if longestCommonPrefixLenRunes(d, q) >= 9 {
		return true
	}
	tok := fd
	if q == tok && strings.HasPrefix(d, tok+" ") {
		return true
	}
	if d == tok && strings.HasPrefix(q, tok+" ") {
		return true
	}
	return false
}

// mfrLeadingBrandKeyFromNorm 从已 NormalizeMfrString 的全串取出「品牌前缀键」：若有 '(' 则取括号前一段的首词，否则取首词。
func mfrLeadingBrandKeyFromNorm(normFull string) string {
	s := strings.TrimSpace(normFull)
	if s == "" {
		return ""
	}
	if i := strings.Index(s, "("); i > 0 {
		s = strings.TrimSpace(s[:i])
	}
	return firstMfrTokenNorm(s)
}

func firstMfrTokenNorm(norm string) string {
	fields := strings.Fields(norm)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func longestCommonPrefixLenRunes(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	n := len(ra)
	if len(rb) < n {
		n = len(rb)
	}
	i := 0
	for i < n && ra[i] == rb[i] {
		i++
	}
	return i
}
