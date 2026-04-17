package data

import (
	"strings"
	"sync"
)

// InprocKV 进程内线程安全 KV，供四表读穿与定时预热。
type InprocKV struct {
	mu sync.RWMutex
	m  map[string]any
}

// NewInprocKV 创建空表。
func NewInprocKV() *InprocKV {
	return &InprocKV{m: make(map[string]any)}
}

// Get 读取；第二个返回值为是否命中。
func (k *InprocKV) Get(key string) (any, bool) {
	if k == nil {
		return nil, false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, false
	}
	k.mu.RLock()
	defer k.mu.RUnlock()
	v, ok := k.m[key]
	return v, ok
}

// Set 写入（覆盖）。
func (k *InprocKV) Set(key string, v any) {
	if k == nil || v == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.m == nil {
		k.m = make(map[string]any)
	}
	k.m[key] = v
}

// Delete 删除单键。
func (k *InprocKV) Delete(key string) {
	if k == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, key)
}

// DeletePrefix 删除所有以 prefix 开头的键（含 prefix 本身若存在）。
func (k *InprocKV) DeletePrefix(prefix string) {
	if k == nil {
		return
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	for kk := range k.m {
		if strings.HasPrefix(kk, prefix) {
			delete(k.m, kk)
		}
	}
}
