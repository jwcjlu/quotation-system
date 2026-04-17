package biz

import "caichip/internal/conf"

const (
	defaultHsResolveSyncTimeoutMs = 8000
	defaultHsAutoAcceptThreshold  = 0.9
	defaultHsResolveMaxCandidates = 3
	defaultHsResolveRetryMax      = 2
)

type HsResolveConfig struct {
	SyncTimeoutMs       int32
	AutoAcceptThreshold float64
	MaxCandidates       int
	ResolveRetryMax     int
}

func NewHsResolveConfig(c *conf.Bootstrap) HsResolveConfig {
	cfg := HsResolveConfig{
		SyncTimeoutMs:       defaultHsResolveSyncTimeoutMs,
		AutoAcceptThreshold: defaultHsAutoAcceptThreshold,
		MaxCandidates:       defaultHsResolveMaxCandidates,
		ResolveRetryMax:     defaultHsResolveRetryMax,
	}
	if c == nil {
		return cfg
	}
	if c.HsResolveSyncTimeoutMs > 0 {
		cfg.SyncTimeoutMs = c.HsResolveSyncTimeoutMs
	}
	if c.HsAutoAcceptThreshold != nil {
		cfg.AutoAcceptThreshold = clampZeroToOne(c.GetHsAutoAcceptThreshold())
	}
	if c.HsResolveMaxCandidates > 0 {
		cfg.MaxCandidates = int(c.HsResolveMaxCandidates)
	}
	if c.HsResolveRetryMax >= 0 {
		cfg.ResolveRetryMax = int(c.HsResolveRetryMax)
	}
	return cfg
}

func clampZeroToOne(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
