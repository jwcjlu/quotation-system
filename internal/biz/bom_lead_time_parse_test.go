package biz

import "testing"

func TestParseLeadDays_EmptyAndNA(t *testing.T) {
	for _, s := range []string{"", "   ", "N/A", "n/a", "-", "--"} {
		_, ok := ParseLeadDays(s, "hqchip")
		if ok {
			t.Fatalf("ParseLeadDays(%q) expected ok=false", s)
		}
	}
}

func TestParseLeadDays_Range(t *testing.T) {
	cases := []struct {
		in       string
		wantDays int
	}{
		{"3-5天", 3},
		{"3~5", 3},
		{"10－20天", 10},
		{"  7 ~ 14 ", 7},
	}
	for _, tc := range cases {
		d, ok := ParseLeadDays(tc.in, "ickey")
		if !ok || d != tc.wantDays {
			t.Fatalf("ParseLeadDays(%q) = (%d,%v), want (%d,true)", tc.in, d, ok, tc.wantDays)
		}
	}
}

func TestParseLeadDays_SingleDay(t *testing.T) {
	d, ok := ParseLeadDays("7天", "find_chips")
	if !ok || d != 7 {
		t.Fatalf("got (%d,%v)", d, ok)
	}
	d, ok = ParseLeadDays("12", "hqchip")
	if !ok || d != 12 {
		t.Fatalf("got (%d,%v)", d, ok)
	}
}

func TestParseLeadDays_SpotMappedPlatform(t *testing.T) {
	d, ok := ParseLeadDays("现货", "hqchip")
	if !ok || d != 0 {
		t.Fatalf("got (%d,%v)", d, ok)
	}
	d, ok = ParseLeadDays("现货", "find_chips")
	if !ok || d != 0 {
		t.Fatalf("got (%d,%v)", d, ok)
	}
}

func TestParseLeadDays_SpotUnknownPlatform(t *testing.T) {
	_, ok := ParseLeadDays("现货", "unknown_vendor")
	if ok {
		t.Fatal("expected ok=false for unknown platform")
	}
}

func TestParseLeadDays_Unrecognized(t *testing.T) {
	_, ok := ParseLeadDays("soon", "hqchip")
	if ok {
		t.Fatal("expected ok=false")
	}
}
