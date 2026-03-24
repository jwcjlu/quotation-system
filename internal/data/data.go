package data

import (
	"strings"

	"caichip/internal/conf"

	"gorm.io/gorm"
)

// Data 承载可选数据库连接及驱动名（用于 SQL 方言）。
type Data struct {
	DB *gorm.DB
	// mysqlSkipLocked：MySQL/MariaDB 是否支持 FOR UPDATE SKIP LOCKED（启动时 SELECT VERSION() 探测）。
	mysqlSkipLocked bool
	dbDriver        string
}

// NewData 打开数据库（DSN 为空时 DB 为 nil，cleanup 为空操作）。
func NewData(c *conf.Bootstrap) (*Data, func(), error) {
	if c == nil {
		return &Data{}, func() {}, nil
	}
	db, cleanup, err := NewDB(c.Data)
	if err != nil {
		return nil, nil, err
	}
	driver := ""
	if c.Data != nil && c.Data.Database != nil {
		driver = strings.TrimSpace(strings.ToLower(c.Data.Database.Driver))
	}
	if driver == "" {
		driver = "mysql"
	}
	mysqlSL := false
	if db != nil {
		mysqlSL = detectMySQLSkipLocked(db)
	}
	return &Data{DB: db, dbDriver: driver, mysqlSkipLocked: mysqlSL}, cleanup, nil
}
