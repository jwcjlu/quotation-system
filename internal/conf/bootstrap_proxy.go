package conf

// BootstrapProxy 与根配置 YAML 中 kuaidaili / proxy_backoff / merge_proxy_retry 键对齐（Kratos Scan 第二次扫入同一 config source）。
// 见 docs/superpowers/specs/2026-03-30-proxy-module-kuaidaili-design.md。
type BootstrapProxy struct {
	Kuaidaili       *KuaidailiConfig       `json:"kuaidaili,omitempty"`
	ProxyBackoff    *ProxyBackoffConfig    `json:"proxy_backoff,omitempty"`
	MergeProxyRetry *MergeProxyRetryConfig `json:"merge_proxy_retry,omitempty"`
}

// KuaidailiConfig 快代理 getdps（实现基于 github.com/kuaidaili/golang-sdk 的 kdl Client；enabled=false 时不创建客户端）。
// sign_type：hmacsha1 推荐；token 且配置了 secret_key 时走 SDK 令牌刷新（进程 cwd 下可能写入 .secret）；
// 仅 secret_token、无 secret_key 时使用与文档一致的静态 signature（兼容 HTTP，非 SDK 令牌接口）。
type KuaidailiConfig struct {
	Enabled     bool    `json:"enabled,omitempty"`
	SecretID    string  `json:"secret_id,omitempty"`
	SignType    string  `json:"sign_type,omitempty"`
	SecretKey   string  `json:"secret_key,omitempty"`
	SecretToken string  `json:"secret_token,omitempty"`
	Num         int32   `json:"num,omitempty"`
	Area        string  `json:"area,omitempty"`
	FAuth       int32   `json:"f_auth,omitempty"`
	MaxQPS      float64 `json:"max_qps,omitempty"`
	BaseURL     string  `json:"base_url,omitempty"`
}

// ProxyBackoffConfig BOM 合并代理退避（与规格 §4.2 默认一致）。
type ProxyBackoffConfig struct {
	BaseSec              int32 `json:"base_sec,omitempty"`
	CapSec               int32 `json:"cap_sec,omitempty"`
	MaxAttempts          int32 `json:"max_attempts,omitempty"`
	WallClockDeadlineSec int32 `json:"wall_clock_deadline_sec,omitempty"`
}

// MergeProxyRetryConfig 后台扫描 t_bom_merge_proxy_wait。
type MergeProxyRetryConfig struct {
	Enabled         bool  `json:"enabled,omitempty"`
	TickIntervalSec int32 `json:"tick_interval_sec,omitempty"`
	BatchLimit      int32 `json:"batch_limit,omitempty"`
}
