package biz

import "testing"

func TestClassifyLineAvailability_ReadyWhenAnyPlatformHasUsableQuote(t *testing.T) {
	got := ClassifyLineAvailability(LineAvailabilityInput{
		LineNo:  7,
		MpnNorm: "STM32",
		Platforms: []PlatformAvailabilityFact{
			{PlatformID: "ickey", TaskState: "no_result", NoData: true, ReasonCode: "NO_DATA", Message: "empty", AutoAttempt: 1, ManualAttempt: 0},
			{PlatformID: "szlcsc", TaskState: "succeeded", HasRawQuote: true, HasUsableQuote: true, AutoAttempt: 2, ManualAttempt: 1},
		},
	})

	if got.Status != LineAvailabilityReady {
		t.Fatalf("status = %q, want %q", got.Status, LineAvailabilityReady)
	}
	if got.ReasonCode != "READY" {
		t.Fatalf("reason code = %q, want READY", got.ReasonCode)
	}
	if got.ResolutionStatus != "open" {
		t.Fatalf("resolution status = %q, want open", got.ResolutionStatus)
	}
	if got.LineNo != 7 || got.MpnNorm != "STM32" {
		t.Fatalf("line identity = (%d, %q), want (7, STM32)", got.LineNo, got.MpnNorm)
	}
	if !got.HasUsableQuote {
		t.Fatal("expected line to report usable quote")
	}
	if got.RawQuotePlatformCount != 1 {
		t.Fatalf("raw quote platform count = %d, want 1", got.RawQuotePlatformCount)
	}
	if got.UsableQuotePlatformCount != 1 {
		t.Fatalf("usable quote platform count = %d, want 1", got.UsableQuotePlatformCount)
	}
	if len(got.PlatformFacts) != 2 {
		t.Fatalf("platform facts = %d, want 2", len(got.PlatformFacts))
	}
}

func TestClassifyLineAvailability_NoDataWhenAllPlatformsExplicitlyNoData(t *testing.T) {
	got := ClassifyLineAvailability(LineAvailabilityInput{
		Platforms: []PlatformAvailabilityFact{
			{PlatformID: "ickey", TaskState: "no_result", NoData: true},
			{PlatformID: "szlcsc", TaskState: "no_result", NoData: true},
		},
	})

	if got.Status != LineAvailabilityNoData {
		t.Fatalf("status = %q, want %q", got.Status, LineAvailabilityNoData)
	}
	if got.ReasonCode != "NO_DATA" {
		t.Fatalf("reason code = %q, want NO_DATA", got.ReasonCode)
	}
	if got.ResolutionStatus != "open" {
		t.Fatalf("resolution status = %q, want open", got.ResolutionStatus)
	}
	if got.RawQuotePlatformCount != 0 {
		t.Fatalf("raw quote platform count = %d, want 0", got.RawQuotePlatformCount)
	}
	if got.UsableQuotePlatformCount != 0 {
		t.Fatalf("usable quote platform count = %d, want 0", got.UsableQuotePlatformCount)
	}
}

func TestClassifyLineAvailability_CollectionUnavailableWhenTerminalFailurePresent(t *testing.T) {
	got := ClassifyLineAvailability(LineAvailabilityInput{
		Platforms: []PlatformAvailabilityFact{
			{PlatformID: "ickey", TaskState: "failed_terminal", CollectionUnavailable: true},
			{PlatformID: "szlcsc", TaskState: "no_result", NoData: true},
		},
	})

	if got.Status != LineAvailabilityCollectionUnavailable {
		t.Fatalf("status = %q, want %q", got.Status, LineAvailabilityCollectionUnavailable)
	}
	if got.ReasonCode != "COLLECTION_UNAVAILABLE" {
		t.Fatalf("reason code = %q, want COLLECTION_UNAVAILABLE", got.ReasonCode)
	}
	if got.ResolutionStatus != "open" {
		t.Fatalf("resolution status = %q, want open", got.ResolutionStatus)
	}
	if got.RawQuotePlatformCount != 0 {
		t.Fatalf("raw quote platform count = %d, want 0", got.RawQuotePlatformCount)
	}
	if got.UsableQuotePlatformCount != 0 {
		t.Fatalf("usable quote platform count = %d, want 0", got.UsableQuotePlatformCount)
	}
}

func TestClassifyLineAvailability_NoMatchAfterFilterWhenRawQuoteExistsWithoutUsableQuote(t *testing.T) {
	got := ClassifyLineAvailability(LineAvailabilityInput{
		Platforms: []PlatformAvailabilityFact{
			{PlatformID: "ickey", TaskState: "succeeded", HasRawQuote: true},
			{PlatformID: "szlcsc", TaskState: "no_result", NoData: true},
		},
	})

	if got.Status != LineAvailabilityNoMatchAfterFilter {
		t.Fatalf("status = %q, want %q", got.Status, LineAvailabilityNoMatchAfterFilter)
	}
	if got.ReasonCode != "NO_MATCH_AFTER_FILTER" {
		t.Fatalf("reason code = %q, want NO_MATCH_AFTER_FILTER", got.ReasonCode)
	}
	if got.ResolutionStatus != "open" {
		t.Fatalf("resolution status = %q, want open", got.ResolutionStatus)
	}
	if got.RawQuotePlatformCount != 1 {
		t.Fatalf("raw quote platform count = %d, want 1", got.RawQuotePlatformCount)
	}
	if got.UsableQuotePlatformCount != 0 {
		t.Fatalf("usable quote platform count = %d, want 0", got.UsableQuotePlatformCount)
	}
}

func TestClassifyLineAvailability_CollectingWhenAnyPlatformNonTerminal(t *testing.T) {
	got := ClassifyLineAvailability(LineAvailabilityInput{
		Platforms: []PlatformAvailabilityFact{
			{PlatformID: "szlcsc", TaskState: "running"},
			{PlatformID: "ickey", TaskState: "succeeded", HasRawQuote: true, HasUsableQuote: true},
		},
	})

	if got.Status != LineAvailabilityCollecting {
		t.Fatalf("status = %q, want %q", got.Status, LineAvailabilityCollecting)
	}
	if got.ReasonCode != "COLLECTING" {
		t.Fatalf("reason code = %q, want COLLECTING", got.ReasonCode)
	}
	if got.ResolutionStatus != "open" {
		t.Fatalf("resolution status = %q, want open", got.ResolutionStatus)
	}
	if got.RawQuotePlatformCount != 1 {
		t.Fatalf("raw quote platform count = %d, want 1", got.RawQuotePlatformCount)
	}
	if got.UsableQuotePlatformCount != 1 {
		t.Fatalf("usable quote platform count = %d, want 1", got.UsableQuotePlatformCount)
	}
}

func TestSummarizeLineAvailability_StrictBlocksOnAnyGap(t *testing.T) {
	summary := SummarizeLineAvailability([]LineAvailability{
		{MpnNorm: "A", Status: LineAvailabilityReady},
		{MpnNorm: "B", Status: LineAvailabilityNoData},
		{MpnNorm: "C", Status: LineAvailabilityCollectionUnavailable},
		{MpnNorm: "D", Status: LineAvailabilityNoMatchAfterFilter},
		{MpnNorm: "E", Status: LineAvailabilityCollecting},
	})

	if summary.LineTotal != 5 {
		t.Fatalf("line total = %d, want 5", summary.LineTotal)
	}
	if summary.ReadyLineCount != 1 {
		t.Fatalf("ready line count = %d, want 1", summary.ReadyLineCount)
	}
	if summary.GapLineCount != 3 {
		t.Fatalf("gap line count = %d, want 3", summary.GapLineCount)
	}
	if summary.NoDataLineCount != 1 {
		t.Fatalf("no data line count = %d, want 1", summary.NoDataLineCount)
	}
	if summary.CollectionUnavailableLineCount != 1 {
		t.Fatalf("collection unavailable line count = %d, want 1", summary.CollectionUnavailableLineCount)
	}
	if summary.NoMatchAfterFilterLineCount != 1 {
		t.Fatalf("no match after filter line count = %d, want 1", summary.NoMatchAfterFilterLineCount)
	}
	if summary.CollectingLineCount != 1 {
		t.Fatalf("collecting line count = %d, want 1", summary.CollectingLineCount)
	}
	if !summary.HasStrictBlockingGap() {
		t.Fatal("expected strict mode to block on final gap")
	}
}

func TestSummarizeLineAvailability_CollectingDoesNotStrictBlock(t *testing.T) {
	summary := SummarizeLineAvailability([]LineAvailability{
		{MpnNorm: "A", Status: LineAvailabilityReady},
		{MpnNorm: "B", Status: LineAvailabilityCollecting},
	})

	if summary.LineTotal != 2 {
		t.Fatalf("line total = %d, want 2", summary.LineTotal)
	}
	if summary.ReadyLineCount != 1 {
		t.Fatalf("ready line count = %d, want 1", summary.ReadyLineCount)
	}
	if summary.GapLineCount != 0 {
		t.Fatalf("gap line count = %d, want 0", summary.GapLineCount)
	}
	if summary.CollectingLineCount != 1 {
		t.Fatalf("collecting line count = %d, want 1", summary.CollectingLineCount)
	}
	if summary.HasStrictBlockingGap() {
		t.Fatal("collecting should not be a strict blocking gap")
	}
}
