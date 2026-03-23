package versionutil

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"1.2.0", "1.2.0"},
		{"  v1.2.0  ", "1.2.0"},
		{"V2.0.1", "2.0.1"},
	}
	for _, tt := range tests {
		if got := Normalize(tt.in); got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEqual(t *testing.T) {
	if !Equal("v1.2.0", "1.2.0") {
		t.Fatal("Equal v1.2.0 vs 1.2.0")
	}
}
