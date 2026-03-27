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
	dsn = mysqlDSNWithParseTime(dsn)
	dsn = mysqlDSNWithReadWriteTimeout(dsn)

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
	sqlDB.SetMaxIdleConns(3)
	// 略缩短生命周期，降低拿到已被服务端掐断的空闲连接的概率（Windows 下驱动不做 syscall 层 connCheck）
	sqlDB.SetConnMaxLifetime(10 * time.Minute)
	sqlDB.SetConnMaxIdleTime(30 * time.Second)

	cleanup := func() { _ = sqlDB.Close() }
	return db, cleanup, nil
}

// mysqlDSNWithParseTime 保证 DSN 含 parseTime=true，使 DATETIME 等能扫描到 time.Time（否则驱动返回 []uint8 会触发 Scan error）。
func mysqlDSNWithParseTime(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return dsn
	}
	if strings.Contains(strings.ToLower(dsn), "parsetime=") {
		return dsn
	}
	if strings.Contains(dsn, "?") {
		return dsn + "&parseTime=true"
	}
	return dsn + "?parseTime=true"
}

// mysqlDSNWithReadWriteTimeout 为未配置时追加 readTimeout/writeTimeout，便于驱动在 liveness 与读写时使用 deadline，
// 更快暴露半开连接（可与 SetConnMaxIdleTime 配合）。
func mysqlDSNWithReadWriteTimeout(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return dsn
	}
	lower := strings.ToLower(dsn)
	if strings.Contains(lower, "readtimeout=") || strings.Contains(lower, "writetimeout=") {
		return dsn
	}
	if strings.Contains(dsn, "?") {
		return dsn + "&readTimeout=60s&writeTimeout=60s"
	}
	return dsn + "?readTimeout=60s&writeTimeout=60s"
}
