package biz

import (
	"reflect"
	"testing"
	"time"

	"caichip/internal/conf"
)

func TestDispatchRetryPolicyFromBootstrap_Defaults(t *testing.T) {
	p := DispatchRetryPolicyFromBootstrap(&conf.Bootstrap{})
	if p.RetryMax != 3 {
		t.Fatalf("expected default retry max 3, got %d", p.RetryMax)
	}
	if !reflect.DeepEqual(p.BackoffSec, []int{60, 300, 900}) {
		t.Fatalf("unexpected default backoff: %+v", p.BackoffSec)
	}
}

func TestDispatchRetryPolicyFromBootstrap_ZeroMaxDisablesRetries(t *testing.T) {
	p := DispatchRetryPolicyFromBootstrap(&conf.Bootstrap{
		Agent: &conf.Agent{
			DispatchRetryMax:        0,
			DispatchRetryBackoffSec: []int32{10, 20},
		},
	})
	if p.RetryMax != 0 {
		t.Fatalf("expected retry max 0, got %d", p.RetryMax)
	}
	if _, ok := p.DelayForFailedAttempt(1); ok {
		t.Fatal("retry should be disabled when dispatch_retry_max=0")
	}
}

func TestDispatchRetryPolicyFromBootstrap_CustomBackoff(t *testing.T) {
	p := DispatchRetryPolicyFromBootstrap(&conf.Bootstrap{
		Agent: &conf.Agent{
			DispatchRetryMax:        4,
			DispatchRetryBackoffSec: []int32{15, 45},
		},
	})
	if !reflect.DeepEqual(p.BackoffSec, []int{15, 45}) {
		t.Fatalf("unexpected custom backoff: %+v", p.BackoffSec)
	}
	d, ok := p.DelayForFailedAttempt(4)
	if !ok || d != 45*time.Second {
		t.Fatalf("attempt 4 should reuse last custom backoff, got %v %v", d, ok)
	}
}

func TestDispatchRetryPolicyFromBootstrap_InvalidBackoffFallsBackToDefaults(t *testing.T) {
	p := DispatchRetryPolicyFromBootstrap(&conf.Bootstrap{
		Agent: &conf.Agent{
			DispatchRetryMax:        3,
			DispatchRetryBackoffSec: []int32{60, 0, -1},
		},
	})
	if !reflect.DeepEqual(p.BackoffSec, []int{60, 300, 900}) {
		t.Fatalf("invalid backoff should fall back to defaults, got %+v", p.BackoffSec)
	}
}

func TestDispatchRetryPolicy_DelayForFailedAttempt(t *testing.T) {
	p := DispatchRetryPolicy{RetryMax: 3, BackoffSec: []int{60, 300}}

	d, ok := p.DelayForFailedAttempt(1)
	if !ok || d != 60*time.Second {
		t.Fatalf("attempt 1 should retry in 60s, got %v %v", d, ok)
	}

	d, ok = p.DelayForFailedAttempt(3)
	if !ok || d != 300*time.Second {
		t.Fatalf("attempt 3 should reuse last backoff, got %v %v", d, ok)
	}

	_, ok = p.DelayForFailedAttempt(4)
	if ok {
		t.Fatal("attempt 4 should be terminal when retry_max=3")
	}
}
