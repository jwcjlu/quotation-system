package biz

import (
	"testing"
	"time"

	"caichip/internal/conf"
)

func TestDelayAfterFailureK_noJitter(t *testing.T) {
	cases := []struct {
		k    int
		want int64
	}{
		{0, 30},
		{1, 60},
		{2, 120},
		{6, 1800},
		{7, 1800},
	}
	for _, tc := range cases {
		d := DelayAfterFailureK(tc.k, 30, 1800, 0)
		if int64(d.Seconds()) != tc.want {
			t.Fatalf("k=%d got %v want %ds", tc.k, d, tc.want)
		}
	}
}

func TestDelayAfterFailureK_withJitter(t *testing.T) {
	d := DelayAfterFailureK(0, 30, 1800, 10)
	if d != 40*time.Second {
		t.Fatalf("%v", d)
	}
}

func TestProxyBackoffFromConf_defaults(t *testing.T) {
	p := ProxyBackoffFromConf(nil)
	d := DefaultProxyBackoffParams()
	if p != d {
		t.Fatalf("%+v vs %+v", p, d)
	}
	p2 := ProxyBackoffFromConf(&conf.ProxyBackoffConfig{BaseSec: 60})
	if p2.BaseSec != 60 {
		t.Fatal(p2.BaseSec)
	}
}
