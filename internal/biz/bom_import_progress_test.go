package biz

import "testing"

func TestIsImportStatusTransitionAllowed(t *testing.T) {
	if !IsImportStatusTransitionAllowed(BOMImportStatusFailed, BOMImportStatusParsing) {
		t.Fatalf("failed -> parsing should be allowed")
	}
	if !IsImportStatusTransitionAllowed(BOMImportStatusFailed, BOMImportStatusReady) {
		t.Fatalf("failed -> ready should be allowed")
	}
	if !IsImportStatusTransitionAllowed(BOMImportStatusReady, BOMImportStatusParsing) {
		t.Fatalf("ready -> parsing should be allowed")
	}
	if IsImportStatusTransitionAllowed(BOMImportStatusParsing, BOMImportStatusIdle) {
		t.Fatalf("transitions to idle should be rejected")
	}
	if !IsImportStatusTransitionAllowed(BOMImportStatusParsing, BOMImportStatusFailed) {
		t.Fatalf("parsing -> failed should be allowed")
	}
}
