package conf

type Bootstrap struct {
	Server   *Server   `yaml:"server"`
	Data     *Data     `yaml:"data"`
	Platform *Platform `yaml:"platform"`
	Agent    *Agent    `yaml:"agent"`
}

type Server struct {
	Http *Server_HTTP `yaml:"http"`
	Grpc *Server_GRPC `yaml:"grpc"`
}

type Server_HTTP struct {
	Addr    string `yaml:"addr"`
	Timeout int    `yaml:"timeout"` // seconds
}

type Server_GRPC struct {
	Addr    string `yaml:"addr"`
	Timeout int    `yaml:"timeout"` // seconds
}

type Data struct {
	Database *Data_Database `yaml:"database"`
	Redis    *Data_Redis    `yaml:"redis"`
}

type Data_Database struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

type Data_Redis struct {
	Addr         string `yaml:"addr"`
	ReadTimeout  int    `yaml:"read_timeout"`  // seconds
	WriteTimeout int    `yaml:"write_timeout"` // seconds
}

// Agent 分布式采集 Agent HTTP API（见 docs/分布式采集Agent需求与架构.md）
type Agent struct {
	Enabled                    bool     `yaml:"enabled"`
	ApiKeys                    []string `yaml:"api_keys"`
	LongPollMaxSec             int      `yaml:"long_poll_max_sec"`            // 服务端单次挂起上限，默认 55
	DefaultTaskHeartbeatSec    int      `yaml:"default_task_heartbeat_sec"`   // 用于 T_offline，默认 10
	OfflineMinSec              int      `yaml:"offline_min_sec"`              // 默认 120
	OfflineHeartbeatMultiplier int      `yaml:"offline_heartbeat_multiplier"` // 默认 6，T_offline=max(min, k*interval)
	DevEnqueueEnabled          bool     `yaml:"dev_enqueue_enabled"`          // 仅开发：POST /api/v1/agent/dev/enqueue 入队任务
}

type Platform struct {
	Ickey  *PlatformConf `yaml:"ickey"`
	Szlcsc *PlatformConf `yaml:"szlcsc"`
	Icgoo  *PlatformConf `yaml:"icgoo"`
}

type PlatformConf struct {
	SearchURL     string `yaml:"search_url"`
	Timeout       int    `yaml:"timeout"`
	CrawlerPath   string `yaml:"crawler_path"`
	CrawlerScript string `yaml:"crawler_script"`
	WorkDir       string `yaml:"work_dir"`
}
