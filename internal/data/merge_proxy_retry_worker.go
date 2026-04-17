package data

import (
	"context"
	"time"

	"caichip/internal/biz"
	"caichip/internal/conf"

	"github.com/go-kratos/kratos/v2/log"
	"gorm.io/gorm"
)

// MergeProxyRetryWorker 扫描 t_bom_merge_proxy_wait 到期行并再次 TryDispatchMergeKey。
type MergeProxyRetryWorker struct {
	db     *gorm.DB
	px     *conf.BootstrapProxy
	log    *log.Helper
	merge  biz.MergeDispatchExecutor
	wait   *BomMergeProxyWaitRepo
	cancel context.CancelFunc
}

// NewMergeProxyRetryWorker 依赖可为 nil；Start 内判断 enabled 与 DB。
func NewMergeProxyRetryWorker(
	px *conf.BootstrapProxy,
	logger log.Logger,
	d *Data,
	merge biz.MergeDispatchExecutor,
	wait *BomMergeProxyWaitRepo,
) *MergeProxyRetryWorker {
	var db *gorm.DB
	if d != nil {
		db = d.DB
	}
	return &MergeProxyRetryWorker{
		db:    db,
		px:    px,
		log:   log.NewHelper(logger),
		merge: merge,
		wait:  wait,
	}
}

func mergeProxyRetryConfig(px *conf.BootstrapProxy) (enabled bool, tickSec, batch int32) {
	if px == nil || px.MergeProxyRetry == nil {
		return false, 0, 0
	}
	m := px.MergeProxyRetry
	if !m.Enabled {
		return false, 0, 0
	}
	tick := m.TickIntervalSec
	if tick <= 0 {
		tick = 5
	}
	b := m.BatchLimit
	if b <= 0 {
		b = 32
	}
	return true, tick, b
}

// Start 非阻塞。
func (w *MergeProxyRetryWorker) Start() {
	if w == nil || w.db == nil || w.merge == nil || w.wait == nil || !w.wait.DBOk() {
		return
	}
	enabled, tickSec, batch := mergeProxyRetryConfig(w.px)
	if !enabled {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	go w.loop(ctx, time.Duration(tickSec)*time.Second, int(batch))
}

// Stop 停止 ticker。
func (w *MergeProxyRetryWorker) Stop() {
	if w == nil || w.cancel == nil {
		return
	}
	w.cancel()
	w.cancel = nil
}

func (w *MergeProxyRetryWorker) loop(ctx context.Context, tick time.Duration, batch int) {
	t := time.NewTicker(tick)
	defer t.Stop()
	w.tickOnce(ctx, batch)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.tickOnce(ctx, batch)
		}
	}
}

func (w *MergeProxyRetryWorker) tickOnce(ctx context.Context, batch int) {
	rows, err := w.wait.ListDue(ctx, batch)
	if err != nil {
		w.log.Errorf("merge_proxy_retry list due: %v", err)
		return
	}
	for _, row := range rows {
		if err := w.merge.TryDispatchMergeKey(ctx, row.MpnNorm, row.PlatformID, row.BizDate); err != nil {
			w.log.Errorf("merge_proxy_retry TryDispatchMergeKey %s/%s: %v", row.MpnNorm, row.PlatformID, err)
		}
	}
}
