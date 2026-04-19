package biz

import "caichip/internal/conf"

const (
	defaultHsResolveSyncTimeoutMs = 8000
	defaultHsAutoAcceptThreshold  = 0.9
	defaultHsResolveMaxCandidates = 3
	defaultHsResolveRetryMax      = 2
	defaultHsManualUploadMaxBytes = 20 << 20 // 20 MiB
	defaultHsManualUploadTTLSec   = 86400    // 24h
	defaultHsManualDescMaxRunes   = 12000
)

type HsResolveConfig struct {
	SyncTimeoutMs       int32
	AutoAcceptThreshold float64
	MaxCandidates       int
	ResolveRetryMax     int
	// 以下为零时使用默认常量。
	ManualUploadMaxBytes      int32
	ManualUploadTTLSeconds    int32
	ManualDescriptionMaxRunes int32
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

// ManualUploadMaxBytesOrDefault 上传 PDF 单文件上限（字节）。
func (c HsResolveConfig) ManualUploadMaxBytesOrDefault() int {
	if c.ManualUploadMaxBytes > 0 {
		return int(c.ManualUploadMaxBytes)
	}
	return defaultHsManualUploadMaxBytes
}

// ManualUploadTTLSecondsOrDefault staging TTL（秒）。
func (c HsResolveConfig) ManualUploadTTLSecondsOrDefault() int {
	if c.ManualUploadTTLSeconds > 0 {
		return int(c.ManualUploadTTLSeconds)
	}
	return defaultHsManualUploadTTLSec
}

// ManualDescriptionMaxRunesOrDefault 手动描述最大 Unicode 标量个数。
func (c HsResolveConfig) ManualDescriptionMaxRunesOrDefault() int {
	if c.ManualDescriptionMaxRunes > 0 {
		return int(c.ManualDescriptionMaxRunes)
	}
	return defaultHsManualDescMaxRunes
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
