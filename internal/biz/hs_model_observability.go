package biz

// HsResolveObserver 抽象 HS 解析链路的日志与指标输出。
type HsResolveObserver interface {
	RecordMetric(name string, value float64, labels ...string)
	EmitLog(event string, fields map[string]any)
}

type noopHsResolveObserver struct{}

func (noopHsResolveObserver) RecordMetric(string, float64, ...string) {}
func (noopHsResolveObserver) EmitLog(string, map[string]any)          {}
