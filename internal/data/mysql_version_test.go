package data

import "testing"

func TestVersionStringSupportsSkipLocked(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		{"", false},
		{"5.7.44", false},
		{"8.0.0", false},
		{"8.0.1", true},
		{"8.0.33", true},
		{"9.0.0", true},
		{"10.6.5-MariaDB", true},
		{"10.5.12-MariaDB", false},
	}
	for _, tc := range cases {
		if got := versionStringSupportsSkipLocked(tc.v); got != tc.want {
			t.Fatalf("%q: got %v want %v", tc.v, got, tc.want)
		}
	}
}
