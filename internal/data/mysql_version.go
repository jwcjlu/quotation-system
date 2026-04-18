package data

import (
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// detectMySQLSkipLocked 在连接建立后探测是否支持 FOR UPDATE SKIP LOCKED。
// 解析失败或查询失败时返回 false，使用仅 FOR UPDATE 以兼容 MySQL 5.7 等。
// 说明：无业务表可映射为 GORM Model 的 VERSION() 标量查询，此处保留最小 Scan。
func detectMySQLSkipLocked(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	var row struct {
		Version string `gorm:"column:version"`
	}
	// 通过 DUAL + Select 获取版本，避免 Raw SQL。
	if err := db.Table("DUAL").Select("VERSION() AS version").Take(&row).Error; err != nil {
		return false
	}
	return versionStringSupportsSkipLocked(row.Version)
}

// versionStringSupportsSkipLocked：MySQL 8.0.1+、MariaDB 10.6+。
func versionStringSupportsSkipLocked(version string) bool {
	v := strings.TrimSpace(version)
	if v == "" {
		return false
	}
	lower := strings.ToLower(v)
	isMaria := strings.Contains(lower, "mariadb")

	i := 0
	for i < len(v) && ((v[i] >= '0' && v[i] <= '9') || v[i] == '.') {
		i++
	}
	if i == 0 {
		return false
	}
	parts := strings.Split(v[:i], ".")
	if len(parts) < 2 {
		return false
	}
	maj, err1 := strconv.Atoi(parts[0])
	min, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	patch := 0
	if len(parts) >= 3 {
		patch, _ = strconv.Atoi(parts[2])
	}

	if isMaria {
		return maj > 10 || (maj == 10 && min >= 6)
	}
	// MySQL: 8.0.1+（8.0.0 无 SKIP LOCKED）
	return maj > 8 || (maj == 8 && (min > 0 || patch >= 1))
}
