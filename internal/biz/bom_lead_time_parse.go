package biz

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	leadRangeRe  = regexp.MustCompile(`^\s*(\d+)\s*[-~～－]\s*(\d+)`)
	leadSingleRe = regexp.MustCompile(`^\s*(\d+)(?:\s*天)?\s*$`)
)

// platformLeadSpotIsZeroDays returns whether "现货" maps to 0 lead days for this platform (V1 table in code).
func platformLeadSpotIsZeroDays(platformID string) bool {
	switch NormalizePlatformID(platformID) {
	case "find_chips", "hqchip", "ickey", "szlcsc":
		return true
	default:
		return false
	}
}

// ParseLeadDays parses a human-readable lead time string into comparable calendar days (§1.10).
// Empty, N/A-like tokens, or unrecognized text → ok false.
// Ranges such as "3-5天" or "3~5" use the lower bound (conservative shorter-lead preference).
// "现货" returns 0, ok true only when platformLeadSpotIsZeroDays(platformID) is true.
func ParseLeadDays(leadTime string, platformID string) (days int, ok bool) {
	s := strings.TrimSpace(leadTime)
	if s == "" {
		return 0, false
	}
	if isLeadTimeNA(s) {
		return 0, false
	}
	if s == "现货" {
		if platformLeadSpotIsZeroDays(platformID) {
			return 0, true
		}
		return 0, false
	}
	if sub := leadRangeRe.FindStringSubmatch(s); len(sub) == 3 {
		lo, err := strconv.Atoi(sub[1])
		if err != nil || lo < 0 {
			return 0, false
		}
		return lo, true
	}
	if sub := leadSingleRe.FindStringSubmatch(s); len(sub) == 2 {
		n, err := strconv.Atoi(sub[1])
		if err != nil || n < 0 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

func isLeadTimeNA(s string) bool {
	t := strings.TrimSpace(strings.ToLower(s))
	switch t {
	case "n/a", "-", "--":
		return true
	default:
		return false
	}
}
