package agentapp

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	apiconst "caichip/api/agent"
	v1 "caichip/api/agent/v1"

	"google.golang.org/protobuf/types/known/structpb"
)

// App Agent 主循环：任务心跳（长轮询）与脚本同步心跳；任务按 script_id 串行、全局并行度受限。
type App struct {
	cfg    *Config
	client *Client
	log    *slog.Logger

	sem         chan struct{}
	scriptMu    sync.Mutex
	scriptLocks map[string]*sync.Mutex
}

// NewApp 构造 Agent（HTTP 使用 Kratos + proto 生成客户端）。
func NewApp(ctx context.Context, cfg *Config) (*App, error) {
	client, err := NewClient(ctx, cfg.BaseURL, cfg.APIKey, cfg.HTTPTimeout)
	if err != nil {
		return nil, err
	}
	return &App{
		cfg:         cfg,
		client:      client,
		log:         slog.Default(),
		sem:         make(chan struct{}, max(1, cfg.MaxParallel)),
		scriptLocks: make(map[string]*sync.Mutex),
	}, nil
}

func (a *App) lockForScript(scriptID string) *sync.Mutex {
	a.scriptMu.Lock()
	defer a.scriptMu.Unlock()
	if a.scriptLocks[scriptID] == nil {
		a.scriptLocks[scriptID] = &sync.Mutex{}
	}
	return a.scriptLocks[scriptID]
}

// Run 阻塞直到 ctx 取消；启动任务心跳与脚本同步两个 goroutine。
func (a *App) Run(ctx context.Context) error {
	if err := os.MkdirAll(a.cfg.DataDir, 0755); err != nil {
		return err
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		a.loopTaskHeartbeat(ctx)
	}()
	go func() {
		defer wg.Done()
		a.loopScriptSync(ctx)
	}()
	a.log.Info("caichip agent started",
		"agent_id", a.cfg.AgentID,
		"base", a.cfg.BaseURL,
		"data_dir", a.cfg.DataDir,
	)
	<-ctx.Done()
	wg.Wait()
	return nil
}

func (a *App) loopTaskHeartbeat(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		scripts, err := ScanInstalledScripts(a.cfg.DataDir)
		if err != nil {
			a.log.Error("scan installed scripts", "err", err)
			a.sleepOrDone(ctx, 3*time.Second)
			continue
		}
		req := &v1.TaskHeartbeatRequest{
			ProtocolVersion:    apiconst.ProtocolVersion,
			AgentId:            a.cfg.AgentID,
			Queue:              a.cfg.Queue,
			Tags:               a.cfg.Tags,
			InstalledScripts:   scripts,
			LongPollTimeoutSec: int32(a.cfg.LongPollSec),
		}
		resp, err := a.client.TaskHeartbeat(ctx, req)
		if err != nil {
			a.log.Error("task heartbeat", "err", err)
			a.sleepOrDone(ctx, 3*time.Second)
			continue
		}
		for _, t := range resp.GetTasks() {
			if t == nil {
				continue
			}
			task := t
			go func() {
				a.sem <- struct{}{}
				defer func() { <-a.sem }()
				a.runOneTask(task)
			}()
		}
		a.sleepOrDone(ctx, 50*time.Millisecond)
	}
}

func (a *App) loopScriptSync(ctx context.Context) {
	lp := a.cfg.LongPollSec
	if lp > 55 {
		lp = 55
	}
	lp32 := int32(lp)
	syncOnce := func() {
		rows, err := ScanScriptRowsForSync(a.cfg.DataDir)
		if err != nil {
			a.log.Error("script sync: scan", "err", err)
			return
		}
		req := &v1.ScriptSyncHeartbeatRequest{
			ProtocolVersion:    apiconst.ProtocolVersion,
			AgentId:            a.cfg.AgentID,
			Queue:              a.cfg.Queue,
			Tags:               a.cfg.Tags,
			Scripts:            rows,
			LongPollTimeoutSec: lp32,
		}
		resp, err := a.client.ScriptSyncHeartbeat(ctx, req)
		if err != nil {
			a.log.Error("script sync heartbeat", "err", err)
			return
		}
		a.applySyncActions(ctx, resp.GetSyncActions())
	}
	syncOnce()
	ticker := time.NewTicker(a.cfg.ScriptSyncSec)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncOnce()
		}
	}
}

func (a *App) runOneTask(task *v1.TaskObject) {
	scriptID := task.GetScriptId()
	mu := a.lockForScript(scriptID)
	mu.Lock()
	defer mu.Unlock()

	status, code, so, se := RunTask(a.cfg.DataDir, task, a.cfg.PythonExecutable)

	resMap := map[string]interface{}{
		"script_id": scriptID,
		"version":   task.GetVersion(),
	}
	if code != nil {
		resMap["exit_code"] = float64(*code)
	}
	resultStruct, err := structpb.NewStruct(resMap)
	if err != nil {
		a.log.Error("task result struct", "err", err)
		return
	}

	payload := &v1.TaskResultRequest{
		ProtocolVersion: apiconst.ProtocolVersion,
		AgentId:         a.cfg.AgentID,
		TaskId:          task.GetTaskId(),
		Status:          status,
		LeaseId:         task.GetLeaseId(),
		Attempt:         1,
		StdoutTail:      so,
		StderrTail:      se,
		Result:          resultStruct,
	}
	err = a.client.TaskResult(context.Background(), payload)
	if err != nil {
		var lc *LeaseConflictError
		if errors.As(err, &lc) {
			a.log.Warn("task result rejected (409)", "detail", lc.Detail)
			return
		}
		a.log.Error("task result", "err", err)
	}
}

func (a *App) sleepOrDone(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
