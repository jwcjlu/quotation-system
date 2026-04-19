package data

import "testing"

func TestIsManualStagingDatasheetPath(t *testing.T) {
	t.Parallel()
	if !isManualStagingDatasheetPath(`C:\tmp\caichip\hs_datasheets\manual_staging\ab12cd.pdf`) {
		t.Fatal("expected windows path under manual_staging to match")
	}
	if !isManualStagingDatasheetPath("/var/caichip/hs_datasheets/manual_staging/ab12cd.pdf") {
		t.Fatal("expected unix path under manual_staging to match")
	}
	if isManualStagingDatasheetPath("/var/caichip/hs_datasheets/sha256dead.pdf") {
		t.Fatal("expected non-staging asset path to be rejected")
	}
	if isManualStagingDatasheetPath("/etc/passwd") {
		t.Fatal("expected non-staging path to be rejected")
	}
}
