package agentapp

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// SetupLogging 将日志输出到 stderr 与 workdir 下 LogDir/agent.log，并 slog.SetDefault。
// 须在 LoadConfig 之后、其余用到日志的逻辑之前调用；返回关闭日志文件的函数。
func SetupLogging(cfg *Config) (func(), error) {
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		return nil, err
	}
	path := filepath.Join(cfg.LogDir, "agent.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	h := slog.NewTextHandler(io.MultiWriter(os.Stderr, f), opts)
	slog.SetDefault(slog.New(h))
	return func() { _ = f.Close() }, nil
}
