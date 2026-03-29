package biz

import (
	"context"
	"testing"
)

type fakeAliasLookup map[string]string

func (m fakeAliasLookup) CanonicalID(ctx context.Context, aliasNorm string) (string, bool, error) {
	if m == nil {
		return "", false, nil
	}
	id, ok := m[aliasNorm]
	return id, ok, nil
}

func TestNormalizeMfrString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"  ", ""},
		{"  Molex  ", "MOLEX"},
		{"molex", "MOLEX"},
		// 全角大写 LATIN（NFKC → 半角）再 ToUpper
		{"ＭＯＬＥＸ", "MOLEX"},
	}
	for _, tc := range tests {
		got := NormalizeMfrString(tc.in)
		if got != tc.want {
			t.Errorf("NormalizeMfrString(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveMfrCanonical(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lk := fakeAliasLookup{"MOLEX": "mfr_molex"}

	t.Run("empty_no_constraint", func(t *testing.T) {
		for _, raw := range []string{"", "   ", "\t"} {
			id, hit, err := ResolveManufacturerCanonical(ctx, raw, lk)
			if err != nil {
				t.Fatal(err)
			}
			if hit || id != "" {
				t.Fatalf("raw %q: want empty,false; got %q,%v", raw, id, hit)
			}
		}
	})

	t.Run("hit", func(t *testing.T) {
		id, hit, err := ResolveManufacturerCanonical(ctx, "  molex ", lk)
		if err != nil {
			t.Fatal(err)
		}
		if !hit || id != "mfr_molex" {
			t.Fatalf("want mfr_molex,true; got %q,%v", id, hit)
		}
	})

	t.Run("miss_strict", func(t *testing.T) {
		id, hit, err := ResolveManufacturerCanonical(ctx, "UNKNOWN", lk)
		if err != nil {
			t.Fatal(err)
		}
		if hit || id != "" {
			t.Fatalf("want empty,false; got %q,%v", id, hit)
		}
	})

	t.Run("nil_lookup", func(t *testing.T) {
		id, hit, err := ResolveManufacturerCanonical(ctx, "molex", nil)
		if err != nil {
			t.Fatal(err)
		}
		if hit || id != "" {
			t.Fatalf("want empty,false; got %q,%v", id, hit)
		}
	})
}
