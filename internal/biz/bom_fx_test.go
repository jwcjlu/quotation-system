package biz

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeFXLookup struct {
	fn func(ctx context.Context, from, to string, date time.Time) (float64, string, string, bool)
}

func (f *fakeFXLookup) Rate(ctx context.Context, from, to string, date time.Time) (float64, string, string, bool) {
	if f.fn != nil {
		return f.fn(ctx, from, to, date)
	}
	return 0, "", "", false
}

func TestFX_ToBaseCCY_sameCurrency(t *testing.T) {
	ctx := context.Background()
	biz := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	base, meta, err := ToBaseCCY(ctx, 12.5, "usd", "USD", biz, time.Time{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if base != 12.5 || meta.Rate != 1 {
		t.Fatalf("got base=%v rate=%v", base, meta.Rate)
	}
	if meta.FxDateSource != FXDateSourceBizDate {
		t.Fatalf("FxDateSource=%q", meta.FxDateSource)
	}
	if !meta.FxDate.Equal(truncateFXDateUTC(biz)) {
		t.Fatalf("FxDate=%v", meta.FxDate)
	}
}

func TestFX_ToBaseCCY_usesBizDateWhenSet(t *testing.T) {
	ctx := context.Background()
	biz := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	req := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	var seen time.Time
	fk := &fakeFXLookup{
		fn: func(_ context.Context, from, to string, date time.Time) (float64, string, string, bool) {
			seen = date
			if from == "USD" && to == "CNY" {
				return 7.2, "v1", "manual", true
			}
			return 0, "", "", false
		},
	}
	base, meta, err := ToBaseCCY(ctx, 10, "USD", "CNY", biz, req, fk)
	if err != nil {
		t.Fatal(err)
	}
	if base != 72 || meta.Rate != 7.2 {
		t.Fatalf("base=%v meta.Rate=%v", base, meta.Rate)
	}
	if !seen.Equal(truncateFXDateUTC(biz)) {
		t.Fatalf("lookup date want %v got %v", truncateFXDateUTC(biz), seen)
	}
	if meta.FxDateSource != FXDateSourceBizDate {
		t.Fatalf("source %q", meta.FxDateSource)
	}
}

func TestFX_ToBaseCCY_fallsBackToRequestDay(t *testing.T) {
	ctx := context.Background()
	req := time.Date(2026, 3, 21, 15, 30, 0, 0, time.FixedZone("CST", 8*3600))
	var seen time.Time
	fk := &fakeFXLookup{
		fn: func(_ context.Context, from, to string, date time.Time) (float64, string, string, bool) {
			seen = date
			return 7.0, "", "ecb", true
		},
	}
	_, meta, err := ToBaseCCY(ctx, 1, "USD", "CNY", time.Time{}, req, fk)
	if err != nil {
		t.Fatal(err)
	}
	want := truncateFXDateUTC(req)
	if !seen.Equal(want) {
		t.Fatalf("lookup date want %v got %v", want, seen)
	}
	if meta.FxDateSource != FXDateSourceRequestDay {
		t.Fatalf("source %q", meta.FxDateSource)
	}
}

func TestFX_ToBaseCCY_directRate(t *testing.T) {
	ctx := context.Background()
	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fk := &fakeFXLookup{
		fn: func(_ context.Context, from, to string, _ time.Time) (float64, string, string, bool) {
			if from == "EUR" && to == "CNY" {
				return 8.0, "t2026", "manual", true
			}
			return 0, "", "", false
		},
	}
	base, meta, err := ToBaseCCY(ctx, 100, "eur", "CNY", d, time.Time{}, fk)
	if err != nil {
		t.Fatal(err)
	}
	if base != 800 || meta.TableVersion != "t2026" {
		t.Fatalf("base=%v tv=%q", base, meta.TableVersion)
	}
}

func TestFX_ToBaseCCY_inverseRate(t *testing.T) {
	ctx := context.Background()
	d := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	// 仅存在 USD→CNY：1 USD = 7 CNY；求 14 CNY → USD 应为 2 USD。
	fk := &fakeFXLookup{
		fn: func(_ context.Context, from, to string, _ time.Time) (float64, string, string, bool) {
			if from == "CNY" && to == "USD" {
				return 0, "", "", false
			}
			if from == "USD" && to == "CNY" {
				return 7, "batch-a", "fixer", true
			}
			return 0, "", "", false
		},
	}
	base, meta, err := ToBaseCCY(ctx, 14, "CNY", "USD", d, time.Time{}, fk)
	if err != nil {
		t.Fatal(err)
	}
	if want := 2.0; base != want {
		t.Fatalf("base=%v want %v", base, want)
	}
	if meta.Rate != 1.0/7.0 {
		t.Fatalf("effective rate %v", meta.Rate)
	}
	if meta.TableVersion != "batch-a" {
		t.Fatalf("tv=%q", meta.TableVersion)
	}
}

func TestFX_ToBaseCCY_ErrFXRateNotFound(t *testing.T) {
	ctx := context.Background()
	d := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	fk := &fakeFXLookup{
		fn: func(_ context.Context, _, _ string, _ time.Time) (float64, string, string, bool) {
			return 0, "", "", false
		},
	}
	_, _, err := ToBaseCCY(ctx, 1, "JPY", "CNY", d, time.Time{}, fk)
	if !errors.Is(err, ErrFXRateNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestFX_ToBaseCCY_inverseZeroRateNotFound(t *testing.T) {
	ctx := context.Background()
	d := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	fk := &fakeFXLookup{
		fn: func(_ context.Context, from, to string, _ time.Time) (float64, string, string, bool) {
			if from == "CNY" && to == "USD" {
				return 0, "", "", false
			}
			if from == "USD" && to == "CNY" {
				return 0, "x", "y", true
			}
			return 0, "", "", false
		},
	}
	_, _, err := ToBaseCCY(ctx, 1, "CNY", "USD", d, time.Time{}, fk)
	if !errors.Is(err, ErrFXRateNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestFX_ToBaseCCY_nilLookupWhenCrossCurrency(t *testing.T) {
	ctx := context.Background()
	d := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	_, _, err := ToBaseCCY(ctx, 1, "USD", "CNY", d, time.Time{}, nil)
	if !errors.Is(err, ErrFXRateNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestFX_ToBaseCCY_directZeroUsesInverse(t *testing.T) {
	ctx := context.Background()
	d := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	fk := &fakeFXLookup{
		fn: func(_ context.Context, from, to string, _ time.Time) (float64, string, string, bool) {
			if from == "EUR" && to == "CNY" {
				return 0, "", "", true
			}
			if from == "CNY" && to == "EUR" {
				return 0.125, "v", "s", true
			}
			return 0, "", "", false
		},
	}
	// 直连 EUR→CNY 为 0；反向 CNY→EUR 0.125 表示 1 CNY = 0.125 EUR ⇒ 1 EUR = 8 CNY。
	base, _, err := ToBaseCCY(ctx, 8, "EUR", "CNY", d, time.Time{}, fk)
	if err != nil {
		t.Fatal(err)
	}
	if want := 64.0; base != want {
		t.Fatalf("base=%v want %v", base, want)
	}
}
