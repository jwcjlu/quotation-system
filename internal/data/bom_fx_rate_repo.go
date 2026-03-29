package data

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"caichip/internal/biz"

	"github.com/go-kratos/kratos/v2/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const bomFxSourceFrankfurter = "frankfurter"

// bomFxBizLocation 北京时间（无夏令时），与常见业务日口径一致。
var bomFxBizLocation = time.FixedZone("CST", 8*3600)

// BomFxRateRepo 配单汇率表 t_bom_fx_rate；实现 biz.FXRateLookup。
// 库中无精确日时，可回源 Frankfurter 拉取并 Upsert，下次直接走库。
type BomFxRateRepo struct {
	db         *gorm.DB
	httpClient *http.Client
	log        *log.Helper
}

// NewBomFxRateRepo 无 DB 时仍返回非 nil，Rate 恒为 !ok。
func NewBomFxRateRepo(db *gorm.DB) *BomFxRateRepo {
	return &BomFxRateRepo{db: db}
}

// NewBomFxRateRepoFromData 供 Wire 注入；与 session/search 一致，无 DB 时 db 为 nil。
// 已连接 DB 时启用 Frankfurter 回源（仅当当日库内无对币种行时请求外网）。
func NewBomFxRateRepoFromData(d *Data, logger log.Logger) *BomFxRateRepo {
	if d == nil || d.DB == nil {
		return &BomFxRateRepo{}
	}
	return &BomFxRateRepo{
		db: d.DB,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		log: log.NewHelper(logger),
	}
}

// DBOk 是否已连接数据库（配单比价汇率查询依赖）。
func (r *BomFxRateRepo) DBOk() bool {
	return r != nil && r.db != nil
}

// Rate 按 **精确 biz_date**（日历日，取北京时间日期部分）匹配一行。
// 若同一 (from,to,date) 下因历史数据或迁移存在多行，取 **id 最大** 的一行（确定性，与单测约定一致）。
// 未命中且已配置 HTTP 客户端时，尝试 Frankfurter 拉取并写入 t_bom_fx_rate 后再查一次。
func (r *BomFxRateRepo) Rate(ctx context.Context, from, to string, date time.Time) (rate float64, tableVersion, source string, ok bool, err error) {
	rate, tableVersion, source, ok, err = r.lookupExact(ctx, from, to, date)
	if err != nil || ok {
		return rate, tableVersion, source, ok, err
	}
	if r.httpClient == nil {
		return 0, "", "", false, nil
	}
	if err := r.fetchAndStoreFrankfurter(ctx, from, to, date); err != nil {
		if r.log != nil {
			r.log.Warnf("bom fx frankfurter %s→%s: %v", strings.ToUpper(strings.TrimSpace(from)), strings.ToUpper(strings.TrimSpace(to)), err)
		}
		return 0, "", "", false, nil
	}
	return r.lookupExact(ctx, from, to, date)
}

func (r *BomFxRateRepo) lookupExact(ctx context.Context, from, to string, date time.Time) (rate float64, tableVersion, source string, ok bool, err error) {
	if r == nil || r.db == nil {
		return 0, "", "", false, nil
	}
	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	if from == "" || to == "" || date.IsZero() {
		return 0, "", "", false, nil
	}
	bizDay := date.In(bomFxBizLocation).Format("2006-01-02")

	var row BomFxRate
	err = r.db.WithContext(ctx).
		Where("from_ccy = ? AND to_ccy = ? AND biz_date = ?", from, to, bizDay).
		Order("id DESC").
		Limit(1).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, "", "", false, nil
	}
	if err != nil {
		return 0, "", "", false, err
	}
	return row.Rate, row.TableVersion, row.Source, true, nil
}

func (r *BomFxRateRepo) fetchAndStoreFrankfurter(ctx context.Context, from, to string, date time.Time) error {
	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	if from == "" || to == "" || date.IsZero() {
		return errors.New("frankfurter: bad args")
	}
	bizDay := truncateBomFxBizDate(date)
	rate, err := fetchFrankfurterRate(ctx, r.httpClient, from, to, bizDay)
	if err != nil {
		return err
	}
	row := BomFxRate{
		FromCcy:      from,
		ToCcy:        to,
		BizDate:      bizDay,
		Rate:         rate,
		Source:       bomFxSourceFrankfurter,
		TableVersion: "",
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "from_ccy"},
			{Name: "to_ccy"},
			{Name: "biz_date"},
			{Name: "source"},
			{Name: "table_version"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"rate", "updated_at"}),
	}).Create(&row).Error
}

// truncateBomFxBizDate 按北京时间取日历日，再规范为 UTC 00:00 写入/匹配 DATE 列（与 Frankfurter 请求路径中的日期串一致）。
func truncateBomFxBizDate(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	y, m, d := t.In(bomFxBizLocation).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

var _ biz.FXRateLookup = (*BomFxRateRepo)(nil)
