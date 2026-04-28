package service

import (
	"context"
	"strings"
	"time"

	"caichip/internal/biz"
)

func (s *BomService) reconcileFinishedDispatchSearchTask(ctx context.Context, sessionID string, bizDate time.Time, row biz.SearchTaskStatusRow) (biz.SearchTaskStatusRow, error) {
	if s == nil || s.search == nil || !s.search.DBOk() {
		return row, nil
	}
	state := biz.NormalizeBOMSearchTaskState(row.SearchTaskState)
	if state != "running" && state != biz.SearchTaskUIStateSearching {
		return row, nil
	}
	if strings.ToLower(strings.TrimSpace(row.DispatchTaskState)) != "finished" {
		return row, nil
	}
	if strings.ToLower(strings.TrimSpace(row.DispatchResult)) != "success" {
		return row, nil
	}

	nextState := "failed_terminal"
	lastErrText := "dispatch finished successfully but stdout quotes were not applied"
	quoteOutcome := ""
	var lastErr *string
	var quotesJSON []byte
	var noMpnDetail []byte

	snap, hit, err := s.search.LoadQuoteCacheByMergeKey(ctx, row.MpnNorm, row.PlatformID, bizDate)
	if err != nil {
		return row, err
	}
	if hit && snap != nil {
		outcome := strings.ToLower(strings.TrimSpace(snap.Outcome))
		if outcome == "no_mpn_match" || outcome == "no_result" {
			nextState = "no_result"
			noMpnDetail = snap.NoMpnDetail
		} else {
			nextState = "succeeded"
			quoteOutcome = strings.TrimSpace(snap.Outcome)
			if quoteOutcome == "" {
				quoteOutcome = "ok"
			}
			quotesJSON = snap.QuotesJSON
			noMpnDetail = snap.NoMpnDetail
		}
		lastErrText = ""
	} else {
		lastErr = &lastErrText
	}

	if err := s.search.FinalizeSearchTask(ctx, sessionID, row.MpnNorm, row.PlatformID, bizDate, row.DispatchTaskID, nextState, lastErr, quoteOutcome, quotesJSON, noMpnDetail); err != nil {
		return row, err
	}
	row.SearchTaskState = nextState
	row.LastError = lastErrText
	if s.session != nil && s.session.DBOk() {
		_ = biz.TryMarkSessionDataReady(ctx, s.session, s.search, sessionID)
	}
	return row, nil
}
