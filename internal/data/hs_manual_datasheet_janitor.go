package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

const hsManualUploadJanitorTick = time.Hour

// HsManualDatasheetJanitor 定时清理过期 staging 行及磁盘文件（上传 TTL 默认 24h，见 biz.HsResolveConfig）。
type HsManualDatasheetJanitor struct {
	repo   *HsManualDatasheetUploadRepo
	log    *log.Helper
	cancel context.CancelFunc
}

// NewHsManualDatasheetJanitor repo 可为 nil 或 DBOk=false 时 Start 为 no-op。
func NewHsManualDatasheetJanitor(repo *HsManualDatasheetUploadRepo, logger log.Logger) *HsManualDatasheetJanitor {
	return &HsManualDatasheetJanitor{
		repo: repo,
		log:  log.NewHelper(logger),
	}
}

// Start 非阻塞；按 hsManualUploadJanitorTick 周期调用 DeleteExpiredBefore(time.Now())。
func (j *HsManualDatasheetJanitor) Start() {
	if j == nil || j.repo == nil || !j.repo.DBOk() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	j.cancel = cancel
	go j.loop(ctx)
}

// Stop 停止后台循环。
func (j *HsManualDatasheetJanitor) Stop() {
	if j == nil || j.cancel == nil {
		return
	}
	j.cancel()
	j.cancel = nil
}

func (j *HsManualDatasheetJanitor) loop(ctx context.Context) {
	t := time.NewTicker(hsManualUploadJanitorTick)
	defer t.Stop()
	j.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			j.runOnce(ctx)
		}
	}
}

func (j *HsManualDatasheetJanitor) runOnce(ctx context.Context) {
	n, err := j.repo.DeleteExpiredBefore(ctx, time.Now().UTC())
	if err != nil {
		if j.log != nil {
			j.log.Errorf("hs_manual_datasheet_janitor: purge failed: %v", err)
		}
		return
	}
	if n > 0 && j.log != nil {
		j.log.Infof("hs_manual_datasheet_janitor: purged %d expired staging row(s)", n)
	}
}
