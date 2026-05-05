package biz

import "strings"

const hsCodeStatusFound = "hs_found"

type HsBatchResolveLineInput struct {
	LineNo       int32
	Model        string
	Manufacturer string
	MatchStatus  string
	HsCodeStatus string
}

type HsBatchResolveLineDecision struct {
	Line   HsBatchResolveLineInput
	Accept bool
	Reason string
}

// DecideBatchResolvableLines 统一批量触发入口判定：
// 1) 仅允许 exact；
// 2) hs_found 跳过；
// 3) model 为空跳过。
func DecideBatchResolvableLines(lines []HsBatchResolveLineInput) []HsBatchResolveLineDecision {
	out := make([]HsBatchResolveLineDecision, 0, len(lines))
	for _, line := range lines {
		decision := HsBatchResolveLineDecision{Line: line}
		matchStatus := strings.TrimSpace(line.MatchStatus)
		hsCodeStatus := strings.TrimSpace(line.HsCodeStatus)
		model := strings.TrimSpace(line.Model)
		switch {
		case model == "":
			decision.Reason = "model is required"
		case matchStatus != "exact":
			decision.Reason = "line is not matched"
		case hsCodeStatus == hsCodeStatusFound:
			decision.Reason = "line already has hs"
		default:
			decision.Accept = true
		}
		out = append(out, decision)
	}
	return out
}
