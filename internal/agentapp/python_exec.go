package agentapp

import (
	"os"
	"runtime"
	"strings"
)

// EffectivePython 返回用于执行采集脚本的解释器命令（与 CAICHIP_PYTHON / 平台默认一致）。
func EffectivePython(pythonExeFromConfig string) string {
	if strings.TrimSpace(pythonExeFromConfig) != "" {
		return strings.TrimSpace(pythonExeFromConfig)
	}
	if p := strings.TrimSpace(os.Getenv("CAICHIP_PYTHON")); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return "python"
	}
	return "python3"
}
