package biz

import "context"

// HsItemListFilter HS 条目分页过滤条件。
type HsItemListFilter struct {
	Page          int32
	PageSize      int32
	CodeTS        string
	GName         string
	SourceCoreHS6 string
}

// HsItemRecord HS 条目只读视图。
type HsItemRecord struct {
	CodeTS        string
	GName         string
	Unit1         string
	Unit2         string
	ControlMark   string
	SourceCoreHS6 string
	RawJSON       []byte
}

// HsItemReadRepo HS 条目只读仓储接口。
type HsItemReadRepo interface {
	DBOk() bool
	List(ctx context.Context, filter HsItemListFilter) ([]HsItemRecord, int64, error)
	GetByCodeTS(ctx context.Context, codeTS string) (*HsItemRecord, error)
	// MapByCodeTS 按 code_ts 批量加载，key 为 trim 后的 code_ts。
	MapByCodeTS(ctx context.Context, codeTSList []string) (map[string]*HsItemRecord, error)
}

// HsItemWriteRepo HS 条目写入仓储接口（按 code_ts upsert）。
type HsItemWriteRepo interface {
	DBOk() bool
	UpsertByCodeTS(ctx context.Context, rows []HsItemRecord) error
}

// HsQueryAPIRepo 第三方 HS 查询接口访问（按 core_hs6 分页拉全量）。
type HsQueryAPIRepo interface {
	FetchAllByCoreHS6(ctx context.Context, coreHS6 string) ([]HsItemRecord, error)
}
