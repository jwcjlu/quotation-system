package service

import (
	"os"
	"strings"
)

// bomMatchTimingEnabled 由环境变量 CAICHIP_BOM_MATCH_TIMING 控制（1/true/yes/on）。
// 开启后 AutoMatch / 就绪计算 / 配单计算会打 Info 级分阶段耗时，便于对照慢查询与行×平台规模。
func bomMatchTimingEnabled() bool {
	switch strings.TrimSpace(strings.ToLower(os.Getenv("CAICHIP_BOM_MATCH_TIMING"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
