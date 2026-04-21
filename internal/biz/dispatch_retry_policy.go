package biz

import (
	"time"

	"caichip/internal/conf"
)

const (
	defaultDispatchRetryMax = 3
)

var defaultDispatchRetryBackoffSec = []int{60, 300, 900}

type DispatchRetryPolicy struct {
	RetryMax   int
	BackoffSec []int
}

func DispatchRetryPolicyFromBootstrap(b *conf.Bootstrap) DispatchRetryPolicy {
	p := DispatchRetryPolicy{
		RetryMax:   defaultDispatchRetryMax,
		BackoffSec: append([]int(nil), defaultDispatchRetryBackoffSec...),
	}
	if b == nil || b.Agent == nil {
		return p
	}
	if b.Agent.DispatchRetryMax >= 0 {
		p.RetryMax = int(b.Agent.DispatchRetryMax)
	}
	if xs := sanitizeDispatchRetryBackoffSec(b.Agent.DispatchRetryBackoffSec); len(xs) > 0 {
		p.BackoffSec = xs
	}
	return p
}

func (p DispatchRetryPolicy) DelayForFailedAttempt(failedAttempt int) (time.Duration, bool) {
	if failedAttempt <= 0 || failedAttempt > p.RetryMax {
		return 0, false
	}
	if len(p.BackoffSec) == 0 {
		return 0, false
	}
	idx := failedAttempt - 1
	if idx >= len(p.BackoffSec) {
		idx = len(p.BackoffSec) - 1
	}
	return time.Duration(p.BackoffSec[idx]) * time.Second, true
}

func sanitizeDispatchRetryBackoffSec(xs []int32) []int {
	if len(xs) == 0 {
		return nil
	}
	out := make([]int, 0, len(xs))
	for _, x := range xs {
		if x > 0 {
			out = append(out, int(x))
			continue
		}
		return nil
	}
	return out
}
