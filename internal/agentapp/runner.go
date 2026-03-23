package agentapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	v1 "caichip/api/agent/v1"
)

const defaultTaskTimeoutSec = 300

// RunTask 使用 Python 解释器执行脚本包内入口；返回 status, exitCode, stdoutTail, stderrTail。
func RunTask(dataDir string, task *v1.TaskObject, pythonExe string) (status string, exitCode *int, stdout, stderr string) {
	if task == nil || task.GetScriptId() == "" || task.GetVersion() == "" {
		return "skipped", nil, "", "missing script_id/version"
	}

	root, err := findVersionRoot(dataDir, task.GetScriptId(), task.GetVersion())
	if err != nil || root == "" {
		return "skipped", nil, "", "script package directory not found"
	}
	vf := filepath.Join(root, "version.txt")
	b, err := os.ReadFile(vf)
	if err != nil {
		return "skipped", nil, "", err.Error()
	}
	localVer := strings.TrimSpace(string(b))
	if !Equal(localVer, task.GetVersion()) {
		return "skipped", nil, "", "version mismatch with version.txt"
	}

	entry := task.GetEntryFile()
	scriptPath := resolveEntry(root, entry)
	if scriptPath == "" {
		return "failed", nil, "", "no entry script (main.py/run.py or entry_file)"
	}

	timeoutSec := defaultTaskTimeoutSec
	if ts := task.GetTimeoutSec(); ts > 0 {
		timeoutSec = int(ts)
	}

	py := EffectivePython(pythonExe)

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

	runErr := cmd.Run()
	so := tail(outBuf.String(), 32000)
	se := tail(errBuf.String(), 32000)

	if ctx.Err() == context.DeadlineExceeded || runErr == context.DeadlineExceeded {
		if cmd.Process != nil {
			killProcessTree(cmd.Process.Pid)
		}
		return "timeout", nil, so, se
	}
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			c := ee.ExitCode()
			return "failed", &c, so, se
		}
		return "failed", nil, so, se + "\n" + runErr.Error()
	}
	c := 0
	return "success", &c, so, se
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
