package data

import "testing"

func TestBOMQuoteCacheCompatColumnsIncludesSourceTracking(t *testing.T) {
	got := bomQuoteCacheCompatColumns()
	want := []string{"SourceType", "SessionID", "LineID", "CreatedBy"}
	if len(got) != len(want) {
		t.Fatalf("columns=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("columns=%v want %v", got, want)
		}
	}
}
