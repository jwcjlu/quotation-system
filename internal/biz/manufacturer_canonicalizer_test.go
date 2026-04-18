package biz

import (
	"context"
	"errors"
	"testing"
)

type canonicalizerAliasLookup struct {
	rows map[string]string
	err  error
}

func (m canonicalizerAliasLookup) CanonicalID(ctx context.Context, aliasNorm string) (string, bool, error) {
	if m.err != nil {
		return "", false, m.err
	}
	if m.rows == nil {
		return "", false, nil
	}
	id, ok := m.rows[aliasNorm]
	return id, ok, nil
}

func TestManufacturerCanonicalizerResolve(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("hit", func(t *testing.T) {
		c := NewManufacturerCanonicalizer(canonicalizerAliasLookup{
			rows: map[string]string{"MOLEX": "mfr_molex"},
		})
		id, hit, err := c.Resolve(ctx, "  molex ")
		if err != nil {
			t.Fatal(err)
		}
		if !hit || id != "mfr_molex" {
			t.Fatalf("want mfr_molex,true; got %q,%v", id, hit)
		}
	})

	t.Run("miss", func(t *testing.T) {
		c := NewManufacturerCanonicalizer(canonicalizerAliasLookup{
			rows: map[string]string{"MOLEX": "mfr_molex"},
		})
		id, hit, err := c.Resolve(ctx, "unknown")
		if err != nil {
			t.Fatal(err)
		}
		if hit || id != "" {
			t.Fatalf("want empty,false; got %q,%v", id, hit)
		}
	})

	t.Run("blank_input", func(t *testing.T) {
		c := NewManufacturerCanonicalizer(canonicalizerAliasLookup{
			rows: map[string]string{"MOLEX": "mfr_molex"},
		})
		id, hit, err := c.Resolve(ctx, " \t ")
		if err != nil {
			t.Fatal(err)
		}
		if hit || id != "" {
			t.Fatalf("want empty,false; got %q,%v", id, hit)
		}
	})

	t.Run("lookup_error", func(t *testing.T) {
		wantErr := errors.New("lookup failed")
		c := NewManufacturerCanonicalizer(canonicalizerAliasLookup{err: wantErr})
		id, hit, err := c.Resolve(ctx, "molex")
		if !errors.Is(err, wantErr) {
			t.Fatalf("want error %v; got %v", wantErr, err)
		}
		if hit || id != "" {
			t.Fatalf("want empty,false when error; got %q,%v", id, hit)
		}
	})

	t.Run("lookup_nil_miss", func(t *testing.T) {
		c := NewManufacturerCanonicalizer(nil)
		id, hit, err := c.Resolve(ctx, "molex")
		if err != nil {
			t.Fatal(err)
		}
		if hit || id != "" {
			t.Fatalf("want empty,false when lookup nil; got %q,%v", id, hit)
		}
	})

	t.Run("nil_receiver_miss", func(t *testing.T) {
		var c *ManufacturerCanonicalizer
		id, hit, err := c.Resolve(ctx, "molex")
		if err != nil {
			t.Fatal(err)
		}
		if hit || id != "" {
			t.Fatalf("want empty,false when receiver nil; got %q,%v", id, hit)
		}
	})

	t.Run("normalize_hit_fullwidth", func(t *testing.T) {
		c := NewManufacturerCanonicalizer(canonicalizerAliasLookup{
			rows: map[string]string{"MOLEX": "mfr_molex"},
		})
		id, hit, err := c.Resolve(ctx, "Ｍｏｌｅｘ")
		if err != nil {
			t.Fatal(err)
		}
		if !hit || id != "mfr_molex" {
			t.Fatalf("want mfr_molex,true; got %q,%v", id, hit)
		}
	})

	t.Run("normalize_hit_compatibility_char", func(t *testing.T) {
		c := NewManufacturerCanonicalizer(canonicalizerAliasLookup{
			rows: map[string]string{"KEMET": "mfr_kemet"},
		})
		id, hit, err := c.Resolve(ctx, "Kemet")
		if err != nil {
			t.Fatal(err)
		}
		if !hit || id != "mfr_kemet" {
			t.Fatalf("want mfr_kemet,true; got %q,%v", id, hit)
		}
	})
}
