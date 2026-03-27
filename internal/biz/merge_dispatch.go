package biz

import (
	"context"
	"time"
)

// MergeDispatchExecutor BOM 合并键调度（设计 §3.5）：缓存短路、在途复用、新抓入队。
type MergeDispatchExecutor interface {
	DBOk() bool
	TryDispatchMergeKey(ctx context.Context, mpnNorm, platformID string, bizDate time.Time) error
	TryDispatchPendingKeysForSession(ctx context.Context, sessionID string) error
}

// MergeKey 合并维度。
type MergeKey struct {
	MpnNorm    string
	PlatformID string
	BizDate    time.Time
}
