package data

import (
	"fmt"
	"strings"
	"time"

	"caichip/internal/conf"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// NewDB 从配置打开 GORM DB。DSN 为空时返回 (nil, noop, nil)。
func NewDB(c *conf.Data) (*gorm.DB, func(), error) {
	if c == nil || c.Database == nil {
		return nil, func() {}, nil
	}
	dsn := strings.TrimSpace(c.Database.Dsn)
	if dsn == "" {
		return nil, func() {}, nil
	}

	dialector := mysql.Open(dsn)

	db, err := gorm.Open(dialector, &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, func() {}, fmt.Errorf("gorm open: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, func() {}, fmt.Errorf("gorm underlying sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	cleanup := func() { _ = sqlDB.Close() }
	return db, cleanup, nil
}
