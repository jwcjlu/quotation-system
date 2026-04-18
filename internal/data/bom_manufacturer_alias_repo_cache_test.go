package data

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countingAliasLookup struct {
	calls atomic.Int32
	hit   bool
	id    string
	err   error
}

func (c *countingAliasLookup) CanonicalID(ctx context.Context, aliasNorm string) (string, bool, error) {
	c.calls.Add(1)
	if c.err != nil {
		return "", false, c.err
	}
	return c.id, c.hit, nil
}

func TestCachedBomManufacturerAliasRepo_CanonicalID_Singleflight(t *testing.T) {
	ctx := context.Background()
	kv := NewInprocKV()
	lookup := &countingAliasLookup{hit: true, id: "mfr_x"}
	r := NewCachedBomManufacturerAliasRepo(&BomManufacturerAliasRepo{}, kv)
	r.alias = lookup

	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, hit, err := r.CanonicalID(ctx, " molex ")
			if err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}
			if !hit || id != "mfr_x" {
				mu.Lock()
				errs = append(errs, fmt.Errorf("want mfr_x,true; got %q,%v", id, hit))
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := lookup.calls.Load(); got != 1 {
		t.Fatalf("expected 1 underlying lookup, got %d", got)
	}
}

func TestCachedBomManufacturerAliasRepo_CanonicalID_TTL_Expires(t *testing.T) {
	oldPos := mfrAliasCanonPosTTL
	oldNeg := mfrAliasCanonNegTTL
	mfrAliasCanonPosTTL = 5 * time.Millisecond
	mfrAliasCanonNegTTL = 5 * time.Millisecond
	t.Cleanup(func() {
		mfrAliasCanonPosTTL = oldPos
		mfrAliasCanonNegTTL = oldNeg
	})

	ctx := context.Background()
	kv := NewInprocKV()
	lookup := &countingAliasLookup{hit: true, id: "mfr_x"}
	r := NewCachedBomManufacturerAliasRepo(&BomManufacturerAliasRepo{}, kv)
	r.alias = lookup

	if _, _, err := r.CanonicalID(ctx, "molex"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.CanonicalID(ctx, "molex"); err != nil {
		t.Fatal(err)
	}
	if got := lookup.calls.Load(); got != 1 {
		t.Fatalf("expected cache hit (1 lookup), got %d", got)
	}

	time.Sleep(8 * time.Millisecond)
	if _, _, err := r.CanonicalID(ctx, "molex"); err != nil {
		t.Fatal(err)
	}
	if got := lookup.calls.Load(); got != 2 {
		t.Fatalf("expected TTL expiry to relookup (2 lookups), got %d", got)
	}
}

func TestCachedBomManufacturerAliasRepo_CanonicalID_ErrorNotCached(t *testing.T) {
	ctx := context.Background()
	kv := NewInprocKV()
	wantErr := errors.New("boom")
	lookup := &countingAliasLookup{err: wantErr}
	r := NewCachedBomManufacturerAliasRepo(&BomManufacturerAliasRepo{}, kv)
	r.alias = lookup

	if _, _, err := r.CanonicalID(ctx, "molex"); !errors.Is(err, wantErr) {
		t.Fatalf("want %v, got %v", wantErr, err)
	}
	lookup.err = nil
	lookup.hit = true
	lookup.id = "mfr_ok"
	if id, hit, err := r.CanonicalID(ctx, "molex"); err != nil || !hit || id != "mfr_ok" {
		t.Fatalf("want ok after error clears, got id=%q hit=%v err=%v", id, hit, err)
	}
	if got := lookup.calls.Load(); got != 2 {
		t.Fatalf("expected two underlying lookups (error path not cached), got %d", got)
	}
}

func TestCachedBomManufacturerAliasRepo_CanonicalID_NegativeCacheTTL(t *testing.T) {
	oldPos := mfrAliasCanonPosTTL
	oldNeg := mfrAliasCanonNegTTL
	mfrAliasCanonPosTTL = 5 * time.Millisecond
	mfrAliasCanonNegTTL = 5 * time.Millisecond
	t.Cleanup(func() {
		mfrAliasCanonPosTTL = oldPos
		mfrAliasCanonNegTTL = oldNeg
	})

	ctx := context.Background()
	kv := NewInprocKV()
	lookup := &countingAliasLookup{hit: false}
	r := NewCachedBomManufacturerAliasRepo(&BomManufacturerAliasRepo{}, kv)
	r.alias = lookup

	if _, hit, err := r.CanonicalID(ctx, "unknown"); err != nil || hit {
		t.Fatalf("want miss, got hit=%v err=%v", hit, err)
	}
	if _, hit, err := r.CanonicalID(ctx, "unknown"); err != nil || hit {
		t.Fatalf("want miss, got hit=%v err=%v", hit, err)
	}
	if got := lookup.calls.Load(); got != 1 {
		t.Fatalf("expected negative cache hit (1 lookup), got %d", got)
	}

	time.Sleep(8 * time.Millisecond)
	if _, hit, err := r.CanonicalID(ctx, "unknown"); err != nil || hit {
		t.Fatalf("want miss after ttl, got hit=%v err=%v", hit, err)
	}
	if got := lookup.calls.Load(); got != 2 {
		t.Fatalf("expected TTL expiry to relookup miss (2 lookups), got %d", got)
	}
}
