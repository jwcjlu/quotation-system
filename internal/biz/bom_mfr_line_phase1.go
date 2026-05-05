package biz

import "strings"

// LinePhase1CleaningSnap 阶段一候选判定用需求行快照（无 DB / 报价 JSON）。
type LinePhase1CleaningSnap struct {
	LineNo                  int
	Mfr                     *string
	ManufacturerCanonicalID *string
}

// LinePhase1CleaningNeed 需出现在阶段一「厂牌待清洗」列表的一行（REQ-S1-001～002）。
type LinePhase1CleaningNeed struct {
	LineNo int
	Mfr    string
}

// SessionLinesNeedingPhase1MfrCleaning 纯函数：norm(mfr) 非空且 manufacturer_canonical_id 为空。
func SessionLinesNeedingPhase1MfrCleaning(lines []LinePhase1CleaningSnap) []LinePhase1CleaningNeed {
	out := make([]LinePhase1CleaningNeed, 0)
	for _, L := range lines {
		raw := strings.TrimSpace(derefPhase1Str(L.Mfr))
		if NormalizeMfrString(raw) == "" {
			continue
		}
		if L.ManufacturerCanonicalID != nil && strings.TrimSpace(*L.ManufacturerCanonicalID) != "" {
			continue
		}
		out = append(out, LinePhase1CleaningNeed{LineNo: L.LineNo, Mfr: raw})
	}
	return out
}

func derefPhase1Str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
