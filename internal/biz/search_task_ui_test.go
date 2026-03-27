package biz

import "testing"

func TestMapSearchTaskStateToQuad(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"pending", SearchUIStatePending},
		{"PENDING", SearchUIStatePending},
		{"dispatched", SearchUISearching},
		{"running", SearchUISearching},
		{"succeeded_quotes", SearchUISucceeded},
		{"succeeded_no_mpn", SearchUISucceeded},
		{"failed", SearchUIFailed},
		{"cancelled", SearchUIFailed},
		{"", SearchUIStatePending},
		{"unknown_xyz", SearchUIStatePending},
	}
	for _, tt := range tests {
		if got := MapSearchTaskStateToQuad(tt.in); got != tt.want {
			t.Errorf("MapSearchTaskStateToQuad(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
