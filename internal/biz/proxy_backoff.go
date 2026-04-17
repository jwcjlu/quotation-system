package biz

import (
	"math"
	"time"

	"caichip/internal/conf"
)

// ProxyBackoffParams 代理重试退避（与规格 §4.2 默认一致）。
type ProxyBackoffParams struct {
	BaseSec              int32
	CapSec               int32
	MaxAttempts          int32
	WallClockDeadlineSec int32
}

// DefaultProxyBackoffParams 规格默认：base 30s、cap 30min、12 次、墙钟 48h。
func DefaultProxyBackoffParams() ProxyBackoffParams {
	return ProxyBackoffParams{
		BaseSec:              30,
		CapSec:               1800,
		MaxAttempts:          12,
		WallClockDeadlineSec: 172800,
	}
}

// ProxyBackoffFromConf 读取配置；nil 或零值字段回退到 DefaultProxyBackoffParams。
func ProxyBackoffFromConf(x *conf.ProxyBackoffConfig) ProxyBackoffParams {
	p := DefaultProxyBackoffParams()
	if x == nil {
		return p
	}
	if x.BaseSec > 0 {
		p.BaseSec = x.BaseSec
	}
	if x.CapSec > 0 {
		p.CapSec = x.CapSec
	}
	if x.MaxAttempts > 0 {
		p.MaxAttempts = x.MaxAttempts
	}
	if x.WallClockDeadlineSec > 0 {
		p.WallClockDeadlineSec = x.WallClockDeadlineSec
	}
	if p.CapSec < p.BaseSec {
		p.CapSec = p.BaseSec
	}
	return p
}

// DelayAfterFailureK 第 k 次失败后的等待秒数（k 从 0 起）：min(cap, base*2^k) + jitterSec。
// jitterSec 应在 [0, baseSec) 内由调用方提供。
func DelayAfterFailureK(k int, baseSec, capSec int64, jitterSec int64) time.Duration {
	if baseSec < 1 {
		baseSec = 1
	}
	if capSec < baseSec {
		capSec = baseSec
	}
	if k < 0 {
		k = 0
	}
	sec := float64(baseSec) * math.Pow(2, float64(k))
	if sec > float64(capSec) || math.IsInf(sec, 0) {
		sec = float64(capSec)
	}
	total := int64(sec) + jitterSec
	if total < 1 {
		total = 1
	}
	return time.Duration(total) * time.Second
}
