package data

import (
	"context"
	"strings"

	"caichip/internal/biz"
)

// CachedBomPlatformScriptRepo 平台配置读穿 + 写后删 bomplat:*。
type CachedBomPlatformScriptRepo struct {
	inner *BomPlatformScriptRepo
	kv    *InprocKV
}

// NewCachedBomPlatformScriptRepo ...
func NewCachedBomPlatformScriptRepo(inner *BomPlatformScriptRepo, kv *InprocKV) *CachedBomPlatformScriptRepo {
	if inner == nil {
		inner = &BomPlatformScriptRepo{}
	}
	if kv == nil {
		kv = NewInprocKV()
	}
	return &CachedBomPlatformScriptRepo{inner: inner, kv: kv}
}

func cloneBomPlatformRows(in []biz.BomPlatformScript) []biz.BomPlatformScript {
	out := make([]biz.BomPlatformScript, len(in))
	copy(out, in)
	for i := range out {
		if len(in[i].RunParamsJSON) > 0 {
			out[i].RunParamsJSON = append([]byte(nil), in[i].RunParamsJSON...)
		}
	}
	return out
}

func cloneBomPlatformPtr(p *biz.BomPlatformScript) *biz.BomPlatformScript {
	if p == nil {
		return nil
	}
	c := *p
	if len(p.RunParamsJSON) > 0 {
		c.RunParamsJSON = append([]byte(nil), p.RunParamsJSON...)
	}
	return &c
}

func (r *CachedBomPlatformScriptRepo) DBOk() bool {
	return r.inner.DBOk()
}

func (r *CachedBomPlatformScriptRepo) List(ctx context.Context) ([]biz.BomPlatformScript, error) {
	key := KeyBomPlatformAll()
	if v, ok := r.kv.Get(key); ok {
		if rows, ok := v.([]biz.BomPlatformScript); ok {
			return cloneBomPlatformRows(rows), nil
		}
	}
	rows, err := r.inner.List(ctx)
	if err != nil {
		return nil, err
	}
	r.kv.Set(key, cloneBomPlatformRows(rows))
	return cloneBomPlatformRows(rows), nil
}

func (r *CachedBomPlatformScriptRepo) Get(ctx context.Context, platformID string) (*biz.BomPlatformScript, error) {
	pid := strings.TrimSpace(platformID)
	if pid == "" {
		return r.inner.Get(ctx, platformID)
	}
	if v, ok := r.kv.Get(KeyBomPlatformAll()); ok {
		if rows, ok := v.([]biz.BomPlatformScript); ok {
			for i := range rows {
				if strings.TrimSpace(rows[i].PlatformID) == pid {
					return cloneBomPlatformPtr(&rows[i]), nil
				}
			}
		}
	}
	oneKey := KeyBomPlatformOne(pid)
	if v, ok := r.kv.Get(oneKey); ok {
		if row, ok := v.(*biz.BomPlatformScript); ok {
			return cloneBomPlatformPtr(row), nil
		}
	}
	p, err := r.inner.Get(ctx, platformID)
	if err != nil {
		return nil, err
	}
	if p != nil {
		r.kv.Set(oneKey, cloneBomPlatformPtr(p))
	}
	return cloneBomPlatformPtr(p), nil
}

func (r *CachedBomPlatformScriptRepo) Upsert(ctx context.Context, p *biz.BomPlatformScript) error {
	if err := r.inner.Upsert(ctx, p); err != nil {
		return err
	}
	r.invalidateBomPlatform(p)
	return nil
}

func (r *CachedBomPlatformScriptRepo) Delete(ctx context.Context, platformID string) error {
	if err := r.inner.Delete(ctx, platformID); err != nil {
		return err
	}
	pid := strings.TrimSpace(platformID)
	r.kv.Delete(KeyBomPlatformAll())
	if pid != "" {
		r.kv.Delete(KeyBomPlatformOne(pid))
	}
	return nil
}

func (r *CachedBomPlatformScriptRepo) invalidateBomPlatform(p *biz.BomPlatformScript) {
	r.kv.Delete(KeyBomPlatformAll())
	if p != nil {
		r.kv.Delete(KeyBomPlatformOne(p.PlatformID))
	}
}

var _ biz.BomPlatformScriptRepo = (*CachedBomPlatformScriptRepo)(nil)
