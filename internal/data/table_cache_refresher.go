package data

import (
	"context"
	"strings"
	"time"

	"caichip/internal/biz"
	"caichip/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"gorm.io/gorm"
)

// TableCacheRefresher 定时全量预热（失败保留旧缓存）；依赖 inner 直读库，不经 Cached 装饰器。
type TableCacheRefresher struct {
	kv         *InprocKV
	db         *gorm.DB
	bc         *conf.Bootstrap
	log        *log.Helper
	platInner  *BomPlatformScriptRepo
	aliasInner *BomManufacturerAliasRepo

	cancel context.CancelFunc
}

// NewTableCacheRefresher refresher 可为 nil 依赖方安全；Start 内再判断 enabled。
func NewTableCacheRefresher(
	kv *InprocKV,
	d *Data,
	bc *conf.Bootstrap,
	logger log.Logger,
	platInner *BomPlatformScriptRepo,
	aliasInner *BomManufacturerAliasRepo,
) *TableCacheRefresher {
	var db *gorm.DB
	if d != nil {
		db = d.DB
	}
	return &TableCacheRefresher{
		kv:         kv,
		db:         db,
		bc:         bc,
		log:        log.NewHelper(logger),
		platInner:  platInner,
		aliasInner: aliasInner,
	}
}

func tableCacheConfig(bc *conf.Bootstrap) (enabled bool, intervalSec int32) {
	if bc == nil || bc.TableCache == nil {
		return false, 0
	}
	return bc.TableCache.Enabled, bc.TableCache.RefreshIntervalSec
}

// Start 非阻塞；enabled=false 或 interval<=0 时仍执行一次 refresh（若 enabled 且希望仅单次，可后续再调）；当前：enabled 且 interval>0 时循环 ticker，并在启动时先 refresh 一次。
func (r *TableCacheRefresher) Start() {
	if r == nil || r.kv == nil {
		return
	}
	enabled, sec := tableCacheConfig(r.bc)
	if !enabled {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	go func() {
		r.refreshOnce(ctx)
		if sec <= 0 {
			return
		}
		t := time.NewTicker(time.Duration(sec) * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.refreshOnce(ctx)
			}
		}
	}()
}

// Stop 取消定时器。
func (r *TableCacheRefresher) Stop() {
	if r == nil || r.cancel == nil {
		return
	}
	r.cancel()
	r.cancel = nil
}

func (r *TableCacheRefresher) refreshOnce(ctx context.Context) {
	if r.kv == nil {
		return
	}
	if r.platInner != nil && r.platInner.DBOk() {
		rows, err := r.platInner.List(ctx)
		if err != nil {
			r.log.Warnf("table_cache: bom_platform list: %v", err)
		} else {
			r.kv.Set(KeyBomPlatformAll(), cloneBomPlatformRows(rows))
		}
	}
	if r.db != nil {
		var auths []CaichipAgentScriptAuth
		if err := r.db.WithContext(ctx).Find(&auths).Error; err != nil {
			r.log.Warnf("table_cache: script_auth scan: %v", err)
		} else {
			byAgent := make(map[string][]biz.AgentScriptAuthSummary)
			for _, row := range auths {
				aid := strings.TrimSpace(row.AgentID)
				if aid == "" {
					continue
				}
				byAgent[aid] = append(byAgent[aid], biz.AgentScriptAuthSummary{
					ScriptID:  strings.TrimSpace(row.ScriptID),
					Username:  strings.TrimSpace(row.Username),
					UpdatedAt: row.UpdatedAt,
				})
			}
			for aid, sums := range byAgent {
				r.kv.Set(KeyAsAuthAgent(aid), cloneAgentScriptSummaries(sums))
			}
		}
		var aliases []BomManufacturerAlias
		if err := r.db.WithContext(ctx).Find(&aliases).Error; err != nil {
			r.log.Warnf("table_cache: manufacturer_alias scan: %v", err)
		} else {
			for _, row := range aliases {
				norm := strings.TrimSpace(row.AliasNorm)
				if norm == "" {
					continue
				}
				canon := strings.TrimSpace(row.CanonicalID)
				r.kv.Set(KeyMfrAliasNorm(norm), &mfrCanonCacheEntry{id: canon, hit: canon != ""})
			}
		}
		var installed []CaichipAgentInstalledScript
		if err := r.db.WithContext(ctx).Find(&installed).Error; err != nil {
			r.log.Warnf("table_cache: installed_script scan: %v", err)
		} else {
			byAgent := make(map[string][]biz.AgentInstalledScriptRow)
			for _, row := range installed {
				aid := strings.TrimSpace(row.AgentID)
				if aid == "" {
					continue
				}
				byAgent[aid] = append(byAgent[aid], biz.AgentInstalledScriptRow{
					ScriptID:  strings.TrimSpace(row.ScriptID),
					Version:   strings.TrimSpace(row.Version),
					EnvStatus: strings.TrimSpace(row.EnvStatus),
					UpdatedAt: row.UpdatedAt,
				})
			}
			for aid, rows := range byAgent {
				r.kv.Set(KeyAgInstAgent(aid), cloneInstalledScriptRows(rows))
			}
		}
	}
	if r.aliasInner != nil && r.aliasInner.DBOk() {
		for _, lim := range []int{300, 1000} {
			rows, err := r.aliasInner.ListDistinctCanonicals(ctx, lim)
			if err != nil {
				r.log.Warnf("table_cache: alias canonicals limit=%d: %v", lim, err)
				continue
			}
			r.kv.Set(KeyMfrAliasCanonicalsList(lim), cloneManufacturerCanonicalRows(rows))
		}
	}
}
