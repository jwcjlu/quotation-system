package data

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"caichip/internal/conf"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
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
	dsn = mysqlDSNWithLoc(dsn, mysqlLocFromConfig(c))

	dialector := mysql.Open(dsn)

	gormCfg := &gorm.Config{SkipDefaultTransaction: true}
	if gormLogSQLEnabled(c) {
		gormCfg.Logger = glogger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			glogger.Config{
				SlowThreshold:             200 * time.Millisecond,
				LogLevel:                  glogger.Info,
				IgnoreRecordNotFoundError: true,
				Colorful:                  false,
			},
		)
	}

	db, err := gorm.Open(dialector, gormCfg)
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

// mysqlDSNWithLoc 追加 go-sql-driver 的 loc（与 parseTime 配合）。未配置时驱动默认 UTC，若库里 DATETIME 存的是本地墙钟（如中国 NOW()），会出现约 8 小时偏差，应设为 Asia/Shanghai 等。
func mysqlDSNWithLoc(dsn, locName string) string {
	dsn = strings.TrimSpace(dsn)
	locName = strings.TrimSpace(locName)
	if dsn == "" || locName == "" {
		return dsn
	}
	lower := strings.ToLower(dsn)
	if strings.Contains(lower, "loc=") {
		return dsn
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "loc=" + url.QueryEscape(locName)
}

func mysqlLocFromConfig(c *conf.Data) string {
	if v := strings.TrimSpace(os.Getenv("CAICHIP_MYSQL_LOC")); v != "" {
		return v
	}
	if c == nil || c.Database == nil {
		return ""
	}
	return strings.TrimSpace(c.Database.GetMysqlLoc())
}

// gormLogSQLEnabled：环境变量 CAICHIP_GORM_LOG_SQL=1/true 优先；否则看 data.database.log_sql。
func gormLogSQLEnabled(c *conf.Data) bool {
	if v := strings.TrimSpace(os.Getenv("CAICHIP_GORM_LOG_SQL")); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	}
	if c == nil || c.Database == nil {
		return false
	}
	return c.Database.GetLogSql()
}
