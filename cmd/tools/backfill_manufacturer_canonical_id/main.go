package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"caichip/internal/biz"
	"caichip/internal/conf"
	"caichip/internal/data"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"gorm.io/gorm"
)

var (
	flagConf   string
	flagDryRun bool
	flagBatch  int
	flagTables string
)

func init() {
	flag.StringVar(&flagConf, "conf", "configs/config.yaml", "config path, eg: -conf configs/config.yaml")
	flag.BoolVar(&flagDryRun, "dry-run", false, "print actions without writing")
	flag.IntVar(&flagBatch, "batch", 500, "batch size (by primary key cursor)")
	flag.StringVar(&flagTables, "tables", "t_bom_quote_item,t_hs_model_mapping,t_hs_model_features,t_hs_model_recommendation", "comma-separated table list")
}

func main() {
	flag.Parse()
	logger := log.NewStdLogger(os.Stdout)
	helper := log.NewHelper(logger)

	c := config.New(config.WithSource(file.NewSource(flagConf)))
	defer c.Close()
	if err := c.Load(); err != nil {
		helper.Fatalf("load config: %v", err)
	}
	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		helper.Fatalf("scan config: %v", err)
	}

	db, cleanup, err := data.NewDB(bc.Data)
	if err != nil {
		helper.Fatalf("open db: %v", err)
	}
	defer cleanup()
	if db == nil {
		helper.Fatalf("database not configured (empty dsn)")
	}

	kv := data.NewInprocKV()
	aliasInner := data.NewBomManufacturerAliasRepo(&data.Data{DB: db})
	alias := data.NewCachedBomManufacturerAliasRepo(aliasInner, kv)

	ctx := context.Background()
	tables := parseTables(flagTables)
	if len(tables) == 0 {
		helper.Fatalf("no tables")
	}

	var totalUpdated int64
	for _, tbl := range tables {
		u, err := backfillTable(ctx, db, alias, tbl, flagBatch, flagDryRun, helper)
		if err != nil {
			helper.Fatalf("backfill %s: %v", tbl, err)
		}
		totalUpdated += u
		helper.Infof("backfill done table=%s updated_rows=%d dry_run=%v", tbl, u, flagDryRun)
	}
	helper.Infof("backfill all done updated_rows=%d dry_run=%v", totalUpdated, flagDryRun)
}

func parseTables(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

type idManufacturerRow struct {
	ID           uint64 `gorm:"column:id"`
	Manufacturer string `gorm:"column:manufacturer"`
}

func backfillTable(ctx context.Context, db *gorm.DB, alias biz.AliasLookup, table string, batch int, dryRun bool, log *log.Helper) (int64, error) {
	if batch <= 0 {
		batch = 500
	}
	table = strings.TrimSpace(table)
	if table == "" {
		return 0, fmt.Errorf("empty table")
	}

	var lastID uint64
	var updated int64
	for {
		var rows []idManufacturerRow
		q := db.WithContext(ctx).
			Table(table).
			Select("id, manufacturer").
			Where("manufacturer <> ''").
			Where("manufacturer_canonical_id IS NULL").
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(batch)
		if err := q.Scan(&rows).Error; err != nil {
			return updated, err
		}
		if len(rows) == 0 {
			break
		}
		lastID = rows[len(rows)-1].ID

		for i := range rows {
			mfr := strings.TrimSpace(rows[i].Manufacturer)
			if mfr == "" {
				continue
			}
			canon, hit, err := biz.ResolveManufacturerCanonical(ctx, mfr, alias)
			if err != nil {
				// 与报价落库一致：基础设施错误不阻断整表回填。
				log.Warnf("table=%s id=%d manufacturer=%q canonical resolve err: %v", table, rows[i].ID, mfr, err)
				continue
			}
			if !hit {
				continue
			}
			if dryRun {
				updated++
				continue
			}
			res := db.WithContext(ctx).
				Table(table).
				Where("id = ? AND manufacturer_canonical_id IS NULL", rows[i].ID).
				Update("manufacturer_canonical_id", canon)
			if res.Error != nil {
				return updated, res.Error
			}
			updated += res.RowsAffected
		}

		// 小让步：避免 tight loop 占满 CPU/DB
		time.Sleep(5 * time.Millisecond)
	}
	return updated, nil
}
