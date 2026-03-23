package agentapp

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Config 从环境变量读取（与 Python agent 对齐）。
type Config struct {
	BaseURL          string
	APIKey           string
	AgentID          string
	Queue            string
	Tags             []string
	DataDir          string
	LongPollSec      int
	HTTPTimeout      time.Duration
	ScriptSyncSec    time.Duration
	MaxParallel      int
	PythonExecutable string // 执行脚本任务的解释器，默认 python/python3
	// PipInstallTimeout 脚本包解压后 pip install -r requirements.txt 的超时（默认 900s）。
	PipInstallTimeout time.Duration
	// SkipPythonCheck 为 true 时跳过启动前 Python/pip 检查（仅建议 CI/容器内自行保证环境）。
	SkipPythonCheck bool
	// AutoInstallPython 默认为 true：无 Python 时会尝试自动安装（Windows: winget 或官网安装包；Linux root: apt；macOS: brew）。
	// 设置 CAICHIP_AUTO_INSTALL_PYTHON=0/false 可关闭。
	AutoInstallPython bool
}

func LoadConfig() (*Config, error) {
	key := strings.TrimSpace(os.Getenv("CAICHIP_API_KEY"))
	if key == "" {
		return nil, fmt.Errorf("CAICHIP_API_KEY is required")
	}
	base := strings.TrimSpace(os.Getenv("CAICHIP_BASE_URL"))
	if base == "" {
		base = "http://127.0.0.1:18080"
	}
	base = strings.TrimRight(base, "/")

	agentID := strings.TrimSpace(os.Getenv("AGENT_ID"))
	if agentID == "" {
		agentID = uuid.NewString()
	}

	tags := []string{}
	if raw := strings.TrimSpace(os.Getenv("AGENT_TAGS")); raw != "" {
		for _, t := range strings.Split(raw, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	dataDir := strings.TrimSpace(os.Getenv("AGENT_DATA_DIR"))
	if dataDir == "" {
		wd, _ := os.Getwd()
		dataDir = wd + string(os.PathSeparator) + "agent_data"
	}

	lp := getenvInt("AGENT_LONG_POLL_SEC", 10)
	httpSec := getenvInt("AGENT_HTTP_TIMEOUT_SEC", 120)
	syncSec := getenvInt("AGENT_SCRIPT_SYNC_SEC", 600)
	maxP := getenvInt("AGENT_MAX_PARALLEL", 4)
	if maxP < 1 {
		maxP = 1
	}
	pipSec := getenvInt("AGENT_PIP_INSTALL_SEC", 900)

	py := strings.TrimSpace(os.Getenv("CAICHIP_PYTHON"))

	queue := strings.TrimSpace(os.Getenv("AGENT_QUEUE"))
	if queue == "" {
		queue = "default"
	}

	skipPy := strings.TrimSpace(os.Getenv("CAICHIP_SKIP_PYTHON_CHECK")) == "1" ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("CAICHIP_SKIP_PYTHON_CHECK")), "true")
	// 默认开启：无 Python 的机器会尝试自动安装；设为 0/false/no/off 可关闭。
	autoPy := getenvBoolDefault("CAICHIP_AUTO_INSTALL_PYTHON", true)

	return &Config{
		BaseURL:           base,
		APIKey:            key,
		AgentID:           agentID,
		Queue:             queue,
		Tags:              tags,
		DataDir:           dataDir,
		LongPollSec:       lp,
		HTTPTimeout:       time.Duration(httpSec) * time.Second,
		ScriptSyncSec:     time.Duration(syncSec) * time.Second,
		MaxParallel:       maxP,
		PythonExecutable:  py,
		PipInstallTimeout: time.Duration(pipSec) * time.Second,
		SkipPythonCheck:   skipPy,
		AutoInstallPython: autoPy,
	}, nil
}

func getenvBoolDefault(key string, def bool) bool {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return def
	}
	switch strings.ToLower(s) {
	case "0", "false", "no", "off":
		return false
	case "1", "true", "yes", "on":
		return true
	default:
		return def
	}
}

func getenvInt(key string, def int) int {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
