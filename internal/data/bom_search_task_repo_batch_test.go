package data

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"caichip/internal/biz"
	"caichip/internal/conf"

	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

func TestBOMSearchTaskRepo_LoadQuoteCachesForKeysUsesBatchQuoteItemQuery(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	driver := strings.TrimSpace(os.Getenv("TEST_DATABASE_DRIVER"))
	if driver == "" {
		driver = "mysql"
	}

	baseDB, cleanup, err := NewDB(&conf.Data{
		Database: &conf.DataDatabase{
			Driver: driver,
			Dsn:    dsn,
		},
	})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer cleanup()
	if err := AutoMigrateSchema(baseDB); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	ctx := context.Background()
	bizDate := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	cacheRows := []BomQuoteCache{
		{MpnNorm: "TDD-BATCH-001", PlatformID: "icgoo", BizDate: bizDate, Outcome: "ok"},
		{MpnNorm: "TDD-BATCH-002", PlatformID: "icgoo", BizDate: bizDate, Outcome: "ok"},
		{MpnNorm: "TDD-BATCH-003", PlatformID: "icgoo", BizDate: bizDate, Outcome: "ok"},
	}
	for i := range cacheRows {
		if err := baseDB.WithContext(ctx).Create(&cacheRows[i]).Error; err != nil {
			t.Fatalf("create quote cache %d: %v", i, err)
		}
	}
	t.Cleanup(func() {
		var ids []uint64
		for _, row := range cacheRows {
			ids = append(ids, row.ID)
		}
		_ = baseDB.WithContext(ctx).Where("quote_id IN ?", ids).Delete(&BomQuoteItem{}).Error
		_ = baseDB.WithContext(ctx).Where("id IN ?", ids).Delete(&BomQuoteCache{}).Error
	})
	for _, row := range cacheRows {
		for itemIdx := 0; itemIdx < 2; itemIdx++ {
			item := BomQuoteItem{
				QuoteID:       row.ID,
				Model:         row.MpnNorm,
				Manufacturer:  "TI",
				Stock:         "1200",
				Package:       "QFN",
				MOQ:           "1",
				MainlandPrice: "1.23",
				QueryModel:    row.MpnNorm,
			}
			if err := baseDB.WithContext(ctx).Create(&item).Error; err != nil {
				t.Fatalf("create quote item for %s: %v", row.MpnNorm, err)
			}
		}
	}

	counter := newCountingGormLogger()
	db := baseDB.Session(&gorm.Session{Logger: counter})
	repo := NewBOMSearchTaskRepo(&Data{DB: db}, nil)

	counter.Reset()
	out, err := repo.LoadQuoteCachesForKeys(ctx, bizDate, []biz.MpnPlatformPair{
		{MpnNorm: "TDD-BATCH-001", PlatformID: "icgoo"},
		{MpnNorm: "TDD-BATCH-002", PlatformID: "icgoo"},
		{MpnNorm: "TDD-BATCH-003", PlatformID: "icgoo"},
	})
	if err != nil {
		t.Fatalf("LoadQuoteCachesForKeys() error = %v", err)
	}
	if got := counter.Count(); got > 2 {
		t.Fatalf("LoadQuoteCachesForKeys() query count = %d, want <= 2", got)
	}
	if len(out) != 3 {
		t.Fatalf("LoadQuoteCachesForKeys() returned %d rows, want 3", len(out))
	}
	for _, row := range cacheRows {
		key := row.MpnNorm + "\x00" + row.PlatformID
		snap := out[key]
		if snap == nil {
			t.Fatalf("missing snapshot for key %q", key)
		}
		var quotes []biz.AgentQuoteRow
		if err := json.Unmarshal(snap.QuotesJSON, &quotes); err != nil {
			t.Fatalf("unmarshal quotes for %q: %v", key, err)
		}
		if len(quotes) != 2 {
			t.Fatalf("quotes len for %q = %d, want 2", key, len(quotes))
		}
	}
}

type countingGormLogger struct {
	glogger.Interface
	count *atomic.Int64
}

func newCountingGormLogger() *countingGormLogger {
	return &countingGormLogger{
		Interface: glogger.Default.LogMode(glogger.Silent),
		count:     &atomic.Int64{},
	}
}

func (l *countingGormLogger) LogMode(level glogger.LogLevel) glogger.Interface {
	return &countingGormLogger{
		Interface: l.Interface.LogMode(level),
		count:     l.count,
	}
}

func (l *countingGormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	l.count.Add(1)
	l.Interface.Trace(ctx, begin, fc, err)
}

func (l *countingGormLogger) Reset() {
	l.count.Store(0)
}

func (l *countingGormLogger) Count() int64 {
	return l.count.Load()
}
