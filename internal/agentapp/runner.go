package agentapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	v1 "caichip/api/agent/v1"
)

const defaultTaskTimeoutSec = 300

// RunTask 使用 Python 解释器执行脚本包内入口；返回 status, exitCode, stdoutTail, stderrTail。
// log 可为 nil，此时使用 slog.Default()。
func RunTask(dataDir string, task *v1.TaskObject, pythonExe string, log *slog.Logger) (status string, exitCode *int, stdout, stderr string) {
	if log == nil {
		log = slog.Default()
	}
	taskID := ""
	if task != nil {
		taskID = task.GetTaskId()
	}
	if task == nil || task.GetScriptId() == "" || task.GetVersion() == "" {
		log.Warn("task skip", "task_id", taskID, "reason", "missing script_id/version")
		return "skipped", nil, "", "missing script_id/version"
	}
	scriptID := task.GetScriptId()
	version := task.GetVersion()

	root, err := findVersionRoot(dataDir, scriptID, version)
	if err != nil || root == "" {
		log.Warn("task skip", "task_id", taskID, "script_id", scriptID, "reason", "no_package_dir", "err", err)
		return "skipped", nil, "", "script package directory not found"
	}
	vf := filepath.Join(root, "version.txt")
	b, err := os.ReadFile(vf)
	if err != nil {
		log.Warn("task skip", "task_id", taskID, "script_id", scriptID, "reason", "read_version_txt", "path", root, "err", err)
		return "skipped", nil, "", err.Error()
	}
	localVer := strings.TrimSpace(string(b))
	if !Equal(localVer, version) {
		log.Warn("task skip", "task_id", taskID, "script_id", scriptID, "reason", "version_mismatch", "local", localVer, "want", version)
		return "skipped", nil, "", "version mismatch with version.txt"
	}

	entry := task.GetEntryFile()
	scriptPath := resolveEntry(root, entry)
	if scriptPath == "" {
		log.Error("task fail", "task_id", taskID, "script_id", scriptID, "reason", "no_entry_script", "root", root)
		return "failed", nil, "", "no entry script (main.py/run.py or entry_file)"
	}

	timeoutSec := defaultTaskTimeoutSec
	if ts := task.GetTimeoutSec(); ts > 0 {
		timeoutSec = int(ts)
	}

	py := EffectivePython(pythonExe)
	entryDisplay := scriptPath
	if rel, err := filepath.Rel(root, scriptPath); err == nil && !strings.HasPrefix(rel, "..") {
		entryDisplay = rel
	}

	params := map[string]any{}
	if p := task.GetParams(); p != nil {
		for k, v := range p.AsMap() {
			params[k] = v
		}
	}
	paramsJSON, _ := json.Marshal(params)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	argv := append([]string{}, task.GetArgv()...)

	args := append([]string{scriptPath}, argv...)
	log.Info("task exec start",
		"task_id", taskID,
		"script_id", scriptID,
		"version", version,
		"cwd", root,
		"entry", entryDisplay,
		"timeout_sec", timeoutSec,
		"py", py,
		"args", args,
	)

	cmd := exec.CommandContext(ctx, py, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"CAICHIP_TASK_PARAMS="+string(paramsJSON),
		"PYTHONUNBUFFERED=1",
	)
	configureCmdProcessGroup(cmd)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	t0 := time.Now()
	runErr := cmd.Run()
	elapsed := time.Since(t0)
	fullOut := outBuf.String()
	fullErr := errBuf.String()
	se := tail(fullErr, 32000)
	outBytes := len([]byte(fullOut))
	errBytes := len([]byte(fullErr))

	if ctx.Err() == context.DeadlineExceeded || runErr == context.DeadlineExceeded {
		log.Warn("task exec timeout", "task_id", taskID, "script_id", scriptID, "timeout_sec", timeoutSec)
		if cmd.Process != nil {
			killProcessTree(cmd.Process.Pid)
		}
		return "timeout", nil, fullOut, se
	}
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			c := ee.ExitCode()
			log.Info("task exec end",
				"task_id", taskID,
				"script_id", scriptID,
				"status", "failed",
				"exit_code", c,
				"duration_ms", elapsed.Milliseconds(),
				"stdout_bytes", outBytes,
				"stderr_bytes", errBytes,
			)
			if strings.TrimSpace(fullErr) != "" {
				log.Warn("task stderr tail", "task_id", taskID, "tail", truncateOneLine(fullErr, 500))
			}
			return "failed", &c, fullOut, se
		}
		log.Info("task exec end",
			"task_id", taskID,
			"script_id", scriptID,
			"status", "failed",
			"exit_code", nil,
			"duration_ms", elapsed.Milliseconds(),
			"stdout_bytes", outBytes,
			"stderr_bytes", errBytes,
			"run_err", runErr.Error(),
		)
		return "failed", nil, fullOut, se + "\n" + runErr.Error()
	}
	c := 0
	log.Info("task exec end",
		"task_id", taskID,
		"script_id", scriptID,
		"status", "success",
		"exit_code", 0,
		"duration_ms", elapsed.Milliseconds(),
		"stdout_bytes", outBytes,
		"stderr_bytes", errBytes,
	)
	return "success", &c, fullOut, se
}

func truncateOneLine(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	r := []rune(s)
	return string(r[:maxRunes]) + "…"
}

func findVersionRoot(dataDir, scriptID, version string) (string, error) {
	base := filepath.Join(dataDir, scriptID)
	ents, err := os.ReadDir(base)
	if err != nil {
		return "", err
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		if Equal(e.Name(), version) {
			return filepath.Join(base, e.Name()), nil
		}
	}
	return "", fmt.Errorf("not found")
}

func resolveEntry(root, entryFile string) string {
	if entryFile != "" {
		p := filepath.Join(root, filepath.Clean(entryFile))
		rel, err := filepath.Rel(filepath.Clean(root), p)
		if err != nil || strings.HasPrefix(rel, "..") {
			return ""
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
		return ""
	}
	for _, name := range []string{"main.py", "run.py"} {
		p := filepath.Join(root, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func tail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}
