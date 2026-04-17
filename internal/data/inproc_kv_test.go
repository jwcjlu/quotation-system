package data

import (
	"sync"
	"testing"
)

func TestInprocKV_DeletePrefix(t *testing.T) {
	k := NewInprocKV()
	k.Set("bomplat:all", 1)
	k.Set("bomplat:icgoo", 2)
	k.Set("other", 3)
	k.DeletePrefix("bomplat:")
	if _, ok := k.Get("bomplat:all"); ok {
		t.Fatal("expected bomplat:all deleted")
	}
	if _, ok := k.Get("bomplat:icgoo"); ok {
		t.Fatal("expected bomplat:icgoo deleted")
	}
	if _, ok := k.Get("other"); !ok {
		t.Fatal("expected other kept")
	}
}

func TestInprocKV_concurrent(t *testing.T) {
	k := NewInprocKV()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				k.Set("k", n+j)
				k.Get("k")
				k.DeletePrefix("p")
			}
		}(i)
	}
	wg.Wait()
}
