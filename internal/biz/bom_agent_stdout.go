package biz

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// ErrBOMQuotesStdoutParseRejected 已命中 bom 行但 stdout 报价 JSON 校验/解析未通过。
var ErrBOMQuotesStdoutParseRejected = errors.New("bom task stdout quotes parse rejected")

// ApplyBOMQuotesFromAgentStdout 在任务成功且 stdout 可解析时，将报价写入缓存并完结所有挂接在同 caichip_task_id 上的 bom_search_task（fan-out）。
// session 可选：非 nil 时对涉及会话调用 TryMarkSessionDataReady。
// 与 BOM 无关、非成功、空 stdout 返回 (false, nil)。命中 BOM 但解析失败返回 (false, ErrBOMQuotesStdoutParseRejected)。
func ApplyBOMQuotesFromAgentStdout(ctx context.Context, repo BOMSearchTaskRepo, session BOMSessionRepo, taskID, status, stdout string) (applied bool, err error) {
	if repo == nil || !repo.DBOk() {
		return false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(status), "success") {
		return false, nil
	}
	if strings.TrimSpace(stdout) == "" {
		return false, nil
	}
	tid := strings.TrimSpace(taskID)
	if tid == "" {
		return false, nil
	}
	rows, err := repo.ListSearchTaskLookupsByCaichipTaskID(ctx, tid)
	if err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}
	quoteRows, ok := ParseTaskStdoutQuoteRows(stdout)
	if !ok {
		msg := "task stdout quotes parse rejected"
		sessions := make(map[string]struct{})
		for _, row := range rows {
			err = repo.FinalizeSearchTask(ctx, row.SessionID, row.MpnNorm, row.PlatformID, row.BizDate, tid, "failed_terminal", &msg, "", nil, nil)
			if err != nil {
				return false, err
			}
			sessions[row.SessionID] = struct{}{}
		}
		if session != nil && session.DBOk() {
			for sid := range sessions {
				_ = TryMarkSessionDataReady(ctx, session, repo, sid)
			}
		}
		return true, nil
	}
	quotes, err := json.Marshal(quoteRows)
	if err != nil {
		return false, ErrBOMQuotesStdoutParseRejected
	}
	sessions := make(map[string]struct{})
	for _, row := range rows {
		err = repo.FinalizeSearchTask(ctx, row.SessionID, row.MpnNorm, row.PlatformID, row.BizDate, tid, "succeeded", nil, "ok", quotes, nil)
		if err != nil {
			return false, err
		}
		sessions[row.SessionID] = struct{}{}
	}
	if session != nil && session.DBOk() {
		for sid := range sessions {
			_ = TryMarkSessionDataReady(ctx, session, repo, sid)
		}
	}
	return true, nil
}
