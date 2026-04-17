package biz

import (
	"testing"

	"caichip/internal/conf"
)

func float64Ptr(v float64) *float64 { return &v }

func TestHsResolveConfig_Defaults(t *testing.T) {
	t.Parallel()
	cfg := NewHsResolveConfig(nil)
	if cfg.SyncTimeoutMs != 8000 {
		t.Fatalf("expected default sync timeout 8000, got %d", cfg.SyncTimeoutMs)
	}
	if cfg.AutoAcceptThreshold != 0.9 {
		t.Fatalf("expected default auto accept threshold 0.9, got %f", cfg.AutoAcceptThreshold)
	}
	if cfg.MaxCandidates != 3 {
		t.Fatalf("expected default max candidates 3, got %d", cfg.MaxCandidates)
	}
	if cfg.ResolveRetryMax != 2 {
		t.Fatalf("expected default retry max 2, got %d", cfg.ResolveRetryMax)
	}
}

func TestHsResolveConfig_Overrides(t *testing.T) {
	t.Parallel()
	cfg := NewHsResolveConfig(&conf.Bootstrap{
		HsResolveSyncTimeoutMs: 3500,
		HsAutoAcceptThreshold:  float64Ptr(0.83),
		HsResolveMaxCandidates: 5,
		HsResolveRetryMax:      4,
	})
	if cfg.SyncTimeoutMs != 3500 {
		t.Fatalf("expected sync timeout 3500, got %d", cfg.SyncTimeoutMs)
	}
	if cfg.AutoAcceptThreshold != 0.83 {
		t.Fatalf("expected threshold 0.83, got %f", cfg.AutoAcceptThreshold)
	}
	if cfg.MaxCandidates != 5 {
		t.Fatalf("expected max candidates 5, got %d", cfg.MaxCandidates)
	}
	if cfg.ResolveRetryMax != 4 {
		t.Fatalf("expected retry max 4, got %d", cfg.ResolveRetryMax)
	}
}

func TestHsResolveConfig_ClampThresholdAndFallback(t *testing.T) {
	t.Parallel()
	cfgLow := NewHsResolveConfig(&conf.Bootstrap{HsAutoAcceptThreshold: float64Ptr(-0.2)})
	if cfgLow.AutoAcceptThreshold != 0 {
		t.Fatalf("expected low threshold clamp to 0, got %f", cfgLow.AutoAcceptThreshold)
	}

	cfgHigh := NewHsResolveConfig(&conf.Bootstrap{HsAutoAcceptThreshold: float64Ptr(1.2)})
	if cfgHigh.AutoAcceptThreshold != 1 {
		t.Fatalf("expected high threshold clamp to 1, got %f", cfgHigh.AutoAcceptThreshold)
	}

	cfgInvalid := NewHsResolveConfig(&conf.Bootstrap{
		HsResolveSyncTimeoutMs: 0,
		HsResolveMaxCandidates: 0,
		HsResolveRetryMax:      -1,
	})
	if cfgInvalid.SyncTimeoutMs != 8000 {
		t.Fatalf("expected invalid timeout fallback to 8000, got %d", cfgInvalid.SyncTimeoutMs)
	}
	if cfgInvalid.MaxCandidates != 3 {
		t.Fatalf("expected invalid max candidates fallback to 3, got %d", cfgInvalid.MaxCandidates)
	}
	if cfgInvalid.ResolveRetryMax != 2 {
		t.Fatalf("expected invalid retry max fallback to 2, got %d", cfgInvalid.ResolveRetryMax)
	}
}

func TestHsResolveConfig_ThresholdUnsetKeepsDefault(t *testing.T) {
	t.Parallel()
	cfg := NewHsResolveConfig(&conf.Bootstrap{
		HsResolveSyncTimeoutMs: 5000,
		HsResolveMaxCandidates: 4,
		HsResolveRetryMax:      1,
	})
	if cfg.AutoAcceptThreshold != 0.9 {
		t.Fatalf("expected default threshold 0.9 when unset, got %f", cfg.AutoAcceptThreshold)
	}
}
