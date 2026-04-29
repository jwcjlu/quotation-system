package logx

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ZapLogger struct {
	core *zap.Logger
}

type Config struct {
	Level       string `json:"level" yaml:"level"`
	Encoding    string `json:"encoding" yaml:"encoding"`
	Development bool   `json:"development" yaml:"development"`
	FilePath    string `json:"file_path" yaml:"file_path"`
	MaxBackups  int    `json:"max_backups" yaml:"max_backups"`
}

// NewZapLogger 创建可供 Kratos 使用的 zap 适配日志器。
func NewZapLogger() (*ZapLogger, error) {
	return NewZapLoggerWithConfig(Config{})
}

// NewZapLoggerWithConfig 根据配置创建可供 Kratos 使用的 zap 适配日志器。
func NewZapLoggerWithConfig(conf Config) (*ZapLogger, error) {
	zapCfg := zap.NewProductionConfig()
	if conf.Development {
		zapCfg = zap.NewDevelopmentConfig()
	}
	zapCfg.Encoding = pickEncoding(conf.Encoding)

	level := zap.NewAtomicLevel()
	if err := level.UnmarshalText([]byte(pickLevel(conf.Level))); err != nil {
		return nil, fmt.Errorf("parse zap level: %w", err)
	}
	zapCfg.Level = level
	zapCfg.EncoderConfig.TimeKey = "ts"
	zapCfg.EncoderConfig.MessageKey = "msg"
	zapCfg.EncoderConfig.CallerKey = "caller"
	zapCfg.EncoderConfig.LevelKey = "level"
	zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	zapCfg.EncoderConfig.EncodeDuration = zapcore.SecondsDurationEncoder

	writeSyncer, err := buildWriteSyncer(conf)
	if err != nil {
		return nil, err
	}
	zapCore := zapcore.NewCore(
		newEncoder(zapCfg.EncoderConfig, pickEncoding(conf.Encoding)),
		writeSyncer,
		level,
	)

	core := zap.New(zapCore, zap.AddCaller(), zap.AddCallerSkip(1))
	if conf.Development {
		core = core.WithOptions(zap.Development())
	}
	return &ZapLogger{core: core}, nil
}

func buildWriteSyncer(conf Config) (zapcore.WriteSyncer, error) {
	logFile := strings.TrimSpace(conf.FilePath)
	if logFile == "" {
		logFile = "logs/server.log"
	}
	dir := filepath.Dir(logFile)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create log dir: %w", err)
		}
	}
	w, err := newRotatingWriter(logFile, 100*1024*1024, pickMaxBackups(conf.MaxBackups))
	if err != nil {
		return nil, err
	}
	return zapcore.AddSync(w), nil
}

func newEncoder(encCfg zapcore.EncoderConfig, encoding string) zapcore.Encoder {
	if encoding == "json" {
		return zapcore.NewJSONEncoder(encCfg)
	}
	return zapcore.NewConsoleEncoder(encCfg)
}

func (z *ZapLogger) Sync() error {
	return z.core.Sync()
}

func (z *ZapLogger) Log(level log.Level, keyvals ...any) error {
	fields := keyvalsToFields(keyvals)
	switch level {
	case log.LevelDebug:
		z.core.Debug("", fields...)
	case log.LevelInfo:
		z.core.Info("", fields...)
	case log.LevelWarn:
		z.core.Warn("", fields...)
	case log.LevelError:
		z.core.Error("", fields...)
	case log.LevelFatal:
		z.core.Fatal("", fields...)
	default:
		z.core.Info("", fields...)
	}
	return nil
}

func keyvalsToFields(keyvals []any) []zap.Field {
	if len(keyvals) == 0 {
		return nil
	}
	fields := make([]zap.Field, 0, len(keyvals)/2+1)
	for i := 0; i < len(keyvals); i += 2 {
		key := "unknown"
		if i < len(keyvals) {
			key = normalizeKey(keyvals[i])
		}
		if i+1 >= len(keyvals) {
			fields = append(fields, zap.Any(key, nil))
			continue
		}
		fields = append(fields, zap.Any(key, keyvals[i+1]))
	}
	return fields
}

func normalizeKey(v any) string {
	switch k := v.(type) {
	case string:
		key := strings.TrimSpace(k)
		if key == "" {
			return "unknown"
		}
		return key
	default:
		return fmt.Sprintf("%v", v)
	}
}

func pickEncoding(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "json" {
		return "json"
	}
	return "console"
}

func pickLevel(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case "debug", "info", "warn", "error", "dpanic", "panic", "fatal":
		return s
	default:
		return "info"
	}
}

func pickMaxBackups(v int) int {
	if v > 0 {
		return v
	}
	return 30
}

type rotatingWriter struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	maxSize int64
	maxKeep int
}

func newRotatingWriter(path string, maxSize int64, maxKeep int) (*rotatingWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	return &rotatingWriter{file: f, path: path, maxSize: maxSize, maxKeep: maxKeep}, nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.rotateIfNeeded(int64(len(p))); err != nil {
		return 0, err
	}
	return w.file.Write(p)
}

func (w *rotatingWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Sync()
}

func (w *rotatingWriter) rotateIfNeeded(nextWrite int64) error {
	st, err := w.file.Stat()
	if err != nil {
		return fmt.Errorf("stat log file: %w", err)
	}
	if st.Size()+nextWrite <= w.maxSize {
		return nil
	}

	if err := w.file.Close(); err != nil {
		return fmt.Errorf("close log file before rotate: %w", err)
	}
	rotated := fmt.Sprintf("%s.%s", w.path, time.Now().Format("20060102-150405"))
	if err := os.Rename(w.path, rotated); err != nil {
		return fmt.Errorf("rename rotated log file: %w", err)
	}
	if err := gzipFile(rotated); err != nil {
		return err
	}
	if err := cleanupOldArchives(w.path, w.maxKeep); err != nil {
		return err
	}

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open new log file after rotate: %w", err)
	}
	w.file = f
	return nil
}

func gzipFile(path string) error {
	src, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open rotated log file: %w", err)
	}
	defer src.Close()

	dstPath := path + ".gz"
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create gzip log file: %w", err)
	}
	gz := gzip.NewWriter(dst)
	if _, err := io.Copy(gz, src); err != nil {
		_ = gz.Close()
		_ = dst.Close()
		return fmt.Errorf("gzip rotated log file: %w", err)
	}
	if err := gz.Close(); err != nil {
		_ = dst.Close()
		return fmt.Errorf("close gzip writer: %w", err)
	}
	if err := dst.Close(); err != nil {
		return fmt.Errorf("close gzip file: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove uncompressed rotated log file: %w", err)
	}
	return nil
}

func cleanupOldArchives(basePath string, maxKeep int) error {
	pattern := basePath + ".*.gz"
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("glob rotated archives: %w", err)
	}
	if len(matches) <= maxKeep {
		return nil
	}

	type archivedFile struct {
		path    string
		modTime time.Time
	}
	archives := make([]archivedFile, 0, len(matches))
	for _, m := range matches {
		st, err := os.Stat(m)
		if err != nil {
			return fmt.Errorf("stat archive file: %w", err)
		}
		archives = append(archives, archivedFile{path: m, modTime: st.ModTime()})
	}
	sort.Slice(archives, func(i, j int) bool {
		return archives[i].modTime.After(archives[j].modTime)
	})

	for _, a := range archives[maxKeep:] {
		if err := os.Remove(a.path); err != nil {
			return fmt.Errorf("remove old archive file: %w", err)
		}
	}
	return nil
}
