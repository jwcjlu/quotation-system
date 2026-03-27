package data

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"

	"gorm.io/gorm"
)

// isRecoverableMySQLError 判断是否为「连接已死 / 半开」类错误，适合在 Ping 后重试一次。
// 说明：在 Windows 上 go-sql-driver/mysql 使用 conncheck_dummy，从池中取出的连接不会做 syscall 层存活检测，
// 远端或 wait_timeout 关掉 TCP 后，首次写仍可能遇到 EOF / invalid connection。
func isRecoverableMySQLError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, driver.ErrBadConn) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "invalid connection") ||
		strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "wsasend") ||
		strings.Contains(msg, "wsarecv")
}

// runGormOnceMoreOnBadConn 执行 op；若为可恢复的连接错误，则 Ping 池子剔除坏连接后重试一次 op。
func runGormOnceMoreOnBadConn(ctx context.Context, db *gorm.DB, op func(*gorm.DB) error) error {
	err := op(db.WithContext(ctx))
	if err == nil || !isRecoverableMySQLError(err) {
		return err
	}
	sqlDB, e := db.DB()
	if e != nil {
		return err
	}
	_ = sqlDB.PingContext(ctx)
	return op(db.WithContext(ctx))
}
