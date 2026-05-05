package biz

import "testing"

func TestSessionLineMfrGateOpen_matchesSessionMfrCleaningGateOpen(t *testing.T) {
	mfr := "TI"
	canon := "MFR_TI"
	lines := []LineMfrGateSnapshot{{LineNo: 1, Mfr: &mfr, ManufacturerCanonicalID: &canon}}
	if g, w := SessionLineMfrGateOpen(lines), SessionMfrCleaningGateOpen(lines); g != w {
		t.Fatalf("SessionLineMfrGateOpen=%v SessionMfrCleaningGateOpen=%v", g, w)
	}
}

func TestSessionMfrCleaningGateOpen_no_lines_is_open(t *testing.T) {
	if !SessionMfrCleaningGateOpen(nil) || !SessionMfrCleaningGateOpen([]LineMfrGateSnapshot{}) {
		t.Fatal("expected gate open with no demand lines")
	}
}

func TestSessionMfrCleaningGateOpen_empty_canonical_string_blocks(t *testing.T) {
	mfr := "TI"
	emptyCanon := ""
	lines := []LineMfrGateSnapshot{
		{LineNo: 1, Mfr: &mfr, ManufacturerCanonicalID: &emptyCanon},
	}
	if SessionMfrCleaningGateOpen(lines) {
		t.Fatal("expected gate closed when canonical is blank string")
	}
}

func TestSessionMfrCleaningGateOpen(t *testing.T) {
	canonTI := "MFR_TI"
	t.Run("all_need_lines_have_canonical", func(t *testing.T) {
		mfr := "TI"
		lines := []LineMfrGateSnapshot{
			{LineNo: 1, Mfr: &mfr, ManufacturerCanonicalID: &canonTI},
		}
		if !SessionMfrCleaningGateOpen(lines) {
			t.Fatal("expected gate open")
		}
	})
	t.Run("missing_canonical_blocks", func(t *testing.T) {
		mfr := "TI"
		lines := []LineMfrGateSnapshot{
			{LineNo: 1, Mfr: &mfr, ManufacturerCanonicalID: nil},
		}
		if SessionMfrCleaningGateOpen(lines) {
			t.Fatal("expected gate closed")
		}
	})
	t.Run("empty_mfr_lines_ignored", func(t *testing.T) {
		mfr := "TI"
		empty := ""
		lines := []LineMfrGateSnapshot{
			{LineNo: 1, Mfr: &empty, ManufacturerCanonicalID: nil},
			{LineNo: 2, Mfr: &mfr, ManufacturerCanonicalID: &canonTI},
		}
		if !SessionMfrCleaningGateOpen(lines) {
			t.Fatal("expected gate open when only nonempty mfr lines need canonical")
		}
	})
}
