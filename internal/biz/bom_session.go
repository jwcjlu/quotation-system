package biz

import (
	"context"
	"errors"
	"time"
)

// ErrBOMSessionNotFound 会话不存在。
var ErrBOMSessionNotFound = errors.New("bom session not found")

// ErrBOMSessionStoreUnavailable 未配置数据库或存储不可用。
var ErrBOMSessionStoreUnavailable = errors.New("bom session store unavailable")

// ErrBOMSessionRevisionConflict 乐观锁：expected_revision 与库中 selection_revision 不一致。
var ErrBOMSessionRevisionConflict = errors.New("bom session selection_revision mismatch")

// ErrBOMSessionPlatformsEmpty 至少选择一个货源 platform_id。
var ErrBOMSessionPlatformsEmpty = errors.New("bom session: at least one platform_id required")

// BOMSession 对应表 bom_session。
type BOMSession struct {
	ID                string
	Title             string
	Status            string
	BizDate           time.Time
	SelectionRevision int
	PlatformIDs       []string
	ParseMode         string
	StorageFileKey    string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// BOMSessionLine 对应表 bom_session_line 的一行。
type BOMSessionLine struct {
	ID        int64
	LineNo    int
	RawText   string
	MPN       string
	MFR       string
	Package   string
	Qty       *float64
	ExtraJSON []byte
}

// BOMSessionRepo 会话持久化。
type BOMSessionRepo interface {
	Create(ctx context.Context, s *BOMSession) error
	GetByID(ctx context.Context, id string) (*BOMSession, error)
	// UpdatePlatformSelection 替换 platform_ids（JSON 全量），并 selection_revision+1。expectedRevision 非 0 时需与当前行一致。
	UpdatePlatformSelection(ctx context.Context, sessionID string, platformIDs []string, expectedRevision int) (newRevision int, err error)
	// ReplaceSessionLines 删除会话下全部行后批量插入，并更新 bom_session.parse_mode。
	ReplaceSessionLines(ctx context.Context, sessionID string, parseMode string, lines []*BOMSessionLine) error
	// ListSessionLines 按 line_no 排序。
	ListSessionLines(ctx context.Context, sessionID string) ([]*BOMSessionLine, error)
	// CountSessionLines 会话下行数。
	CountSessionLines(ctx context.Context, sessionID string) (int, error)
}

// BOMSearchTaskEnsurer 上传/变更会话后为 (MPN×平台×业务日) 写入 bom_search任务（无 DB 时 no-op）。
type BOMSearchTaskEnsurer interface {
	EnsureTasksForSession(ctx context.Context, sessionID string) error
}

// BOMSessionUseCase 会话用例。
type BOMSessionUseCase struct {
	repo BOMSessionRepo
}

// NewBOMSessionUseCase 创建会话用例。
func NewBOMSessionUseCase(repo BOMSessionRepo) *BOMSessionUseCase {
	return &BOMSessionUseCase{repo: repo}
}

// CreateSession 创建草稿会话，业务日为当前 UTC 日期。
func (uc *BOMSessionUseCase) CreateSession(ctx context.Context, title string, platformIDs []string) (*BOMSession, error) {
	if platformIDs == nil {
		platformIDs = []string{}
	}
	now := time.Now().UTC()
	bizDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	s := &BOMSession{
		Title:             title,
		Status:            "draft",
		BizDate:           bizDate,
		SelectionRevision: 1,
		PlatformIDs:       platformIDs,
	}
	if err := uc.repo.Create(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

// GetSession 按 ID 查询。
func (uc *BOMSessionUseCase) GetSession(ctx context.Context, id string) (*BOMSession, error) {
	return uc.repo.GetByID(ctx, id)
}

// ListSessionLines 会话下行列表（导出用）。
func (uc *BOMSessionUseCase) ListSessionLines(ctx context.Context, sessionID string) ([]*BOMSessionLine, error) {
	return uc.repo.ListSessionLines(ctx, sessionID)
}

// CountSessionLines 会话下行数。
func (uc *BOMSessionUseCase) CountSessionLines(ctx context.Context, sessionID string) (int, error) {
	return uc.repo.CountSessionLines(ctx, sessionID)
}

// PutPlatforms 更新勾选平台（全量替换 platform_ids），返回新的 selection_revision。
func (uc *BOMSessionUseCase) PutPlatforms(ctx context.Context, sessionID string, platformIDs []string, expectedRevision int) (int, error) {
	return uc.repo.UpdatePlatformSelection(ctx, sessionID, platformIDs, expectedRevision)
}
