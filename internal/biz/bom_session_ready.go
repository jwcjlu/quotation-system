package biz

import (
	"context"
	"strings"
)

// TryMarkSessionDataReady 若当前任务快照满足 readiness_mode，则更新 bom_session.status 为 data_ready；严格模式不满足时标为 blocked。
func TryMarkSessionDataReady(ctx context.Context, sess BOMSessionRepo, search BOMSearchTaskRepo, sessionID string) error {
	if sess == nil || search == nil || !sess.DBOk() || !search.DBOk() {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	view, err := sess.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	lines, err := sess.ListSessionLines(ctx, sessionID)
	if err != nil {
		return err
	}
	tasks, err := search.ListTasksForSession(ctx, sessionID)
	if err != nil {
		return err
	}
	lineSnaps := make([]LineReadinessSnapshot, 0, len(lines))
	for _, ln := range lines {
		mn := NormalizeMPNForBOMSearch(ln.Mpn)
		lineSnaps = append(lineSnaps, LineReadinessSnapshot{MpnNorm: mn})
	}
	lenientOK := ReadinessFromTasks(ReadinessLenient, tasks, lineSnaps, view.PlatformIDs)
	if !lenientOK {
		return nil
	}
	mode := strings.TrimSpace(view.ReadinessMode)
	if mode == "" {
		mode = ReadinessLenient
	}
	if mode == ReadinessStrict {
		if !ReadinessFromTasks(ReadinessStrict, tasks, lineSnaps, view.PlatformIDs) {
			return sess.SetSessionStatus(ctx, sessionID, "blocked")
		}
	}
	return sess.SetSessionStatus(ctx, sessionID, "data_ready")
}

// NormalizeMPNForBOMSearch 与 data.normalizeMPNForSearchTask 一致，供 biz 侧就绪/任务键对齐。
func NormalizeMPNForBOMSearch(mpn string) string {
	m := strings.TrimSpace(mpn)
	if m == "" {
		return "-"
	}
	return strings.ToUpper(m)
}
