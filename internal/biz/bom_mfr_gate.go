package biz

import "strings"

// LineMfrGateSnapshot 闸门判定用需求行快照（无 data 依赖）。
type LineMfrGateSnapshot struct {
	LineNo                   int
	Mfr                      *string
	ManufacturerCanonicalID *string
}

// SessionLineMfrGateOpen 与 SessionMfrCleaningGateOpen 等价（REQ-GATE-001～003）。
// REQ-GATE-003/004：父行 mfr 为空不参与「须填齐 canonical」的判定（不阻塞闸门）；子报价入阶段二列表时由入队规则单独跳过 mfr 空父行（读模型优先，见 QuoteItemEligibleForPhase2ReviewList 调用方）。
func SessionLineMfrGateOpen(lines []LineMfrGateSnapshot) bool {
	return SessionMfrCleaningGateOpen(lines)
}

// SessionMfrCleaningGateOpen 当且仅当：凡需求厂牌非空行均已具备 manufacturer_canonical_id。
func SessionMfrCleaningGateOpen(lines []LineMfrGateSnapshot) bool {
	for _, line := range lines {
		if NormalizeMfrString(strings.TrimSpace(derefGateStr(line.Mfr))) == "" {
			continue
		}
		if line.ManufacturerCanonicalID == nil || strings.TrimSpace(*line.ManufacturerCanonicalID) == "" {
			return false
		}
	}
	return true
}

func derefGateStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
