package data

import "testing"

func TestSnapshotRows_Empty(t *testing.T) {
	r := &BOMSearchTaskRepo{}
	out := r.snapshotRows(nil)
	if out != nil && len(out) != 0 {
		t.Fatalf("expected empty slice, got %v", out)
	}
}

func TestSnapshotRows_MapsState(t *testing.T) {
	r := &BOMSearchTaskRepo{}
	out := r.snapshotRows([]BomSearchTask{
		{MpnNorm: "ABC", PlatformID: "ickey", State: "Running"},
	})
	if len(out) != 1 || out[0].State != "running" || out[0].MpnNorm != "ABC" {
		t.Fatalf("got %+v", out)
	}
}
