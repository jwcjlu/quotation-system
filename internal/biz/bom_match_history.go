package biz

import (
	"context"
	"errors"
	"time"
)

// ErrMatchHistoryNotFound 历史记录不存在。
var ErrMatchHistoryNotFound = errors.New("match history not found")

// MatchHistoryRow 列表项。
type MatchHistoryRow struct {
	ID          int64
	SessionID   string
	Version     int
	Strategy    string
	CreatedAt   time.Time
	TotalAmount float64
}

// MatchHistoryDetail 详情（含快照 JSON 解析后的 items）。
type MatchHistoryDetail struct {
	ID          int64
	SessionID   string
	Version     int
	Strategy    string
	CreatedAt   time.Time
	TotalAmount float64
	Items       []*MatchItem
}

// BOMMatchHistoryRepo 配单快照持久化（表 bom_match_result）。
type BOMMatchHistoryRepo interface {
	SaveSnapshot(ctx context.Context, sessionID string, strategy string, payloadJSON []byte) error
	List(ctx context.Context, offset, limit int) ([]*MatchHistoryRow, int, error)
	GetByID(ctx context.Context, id int64) (*MatchHistoryDetail, error)
}
