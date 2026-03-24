package agentapp

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	v1 "caichip/api/agent/v1"

	"caichip/internal/pkg/versionutil"
)

// applySyncActions 执行服务端下发的 sync_actions（download / delete），按 script_id 加锁串行。
func (a *App) applySyncActions(ctx context.Context, actions []*v1.SyncAction) {
	n := 0
	for _, act := range actions {
		if act != nil {
			n++
		}
	}
	if n > 0 {
		a.log.Info("sync: applying actions", "count", n)
	}
	for _, act := range actions {
		if act == nil {
			continue
		}
		sid := act.GetScriptId()
		mu := a.lockForScript(sid)
		func() {
			mu.Lock()
			defer mu.Unlock()
			action := strings.ToLower(strings.TrimSpace(act.GetAction()))
			var err error
			switch action {
			case "download":
				a.log.Info("sync: download", "script_id", sid, "version", act.GetVersion())
				err = downloadAndInstallPackage(ctx, a.log, a.cfg, act)
			case "delete":
				a.log.Info("sync: delete", "script_id", sid, "version", act.GetVersion())
				err = deleteScriptVersion(a.cfg.DataDir, act)
			default:
				a.log.Warn("sync: unknown action", "action", act.GetAction(), "script_id", sid)
			}
			if err != nil {
				a.log.Error("sync action failed", "action", act.GetAction(), "script_id", sid, "version", act.GetVersion(), "err", err)
			}
		}()
	}
}

func safeVersionDirName(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "_unknown"
	}
	v = strings.ReplaceAll(v, string(os.PathSeparator), "_")
	v = strings.ReplaceAll(v, "/", "_")
	switch v {
	case ".", "..":
		return "_" + v
	default:
		return v
	}
}

func deleteScriptVersion(dataDir string, act *v1.SyncAction) error {
	scriptID := act.GetScriptId()
	ver := act.GetVersion()
	parent := filepath.Join(dataDir, scriptID)
	ents, err := os.ReadDir(parent)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		if versionutil.Equal(e.Name(), ver) {
			return os.RemoveAll(filepath.Join(parent, e.Name()))
		}
	}
	return nil
}

func downloadAndInstallPackage(ctx context.Context, log *slog.Logger, cfg *Config, act *v1.SyncAction) error {
	dl := act.GetDownload()
	if dl == nil {
		return fmt.Errorf("download: missing download spec")
	}
	url := strings.TrimSpace(dl.GetUrl())
	if url == "" {
		return fmt.Errorf("download: empty url")
	}
	wantSHA := strings.TrimSpace(act.GetPackageSha256())
	if wantSHA == "" {
		return fmt.Errorf("download: empty package_sha256")
	}
	scriptID := act.GetScriptId()
	if strings.TrimSpace(scriptID) == "" {
		return fmt.Errorf("download: empty script_id")
	}
	ver := act.GetVersion()
	if strings.TrimSpace(ver) == "" {
		return fmt.Errorf("download: empty version")
	}

	versionDir := filepath.Join(cfg.DataDir, scriptID, safeVersionDirName(ver))
	_ = os.RemoveAll(versionDir)
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return fmt.Errorf("mkdir version dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, MarkerPreparing), []byte{}, 0644); err != nil {
		return fmt.Errorf("marker preparing: %w", err)
	}

	failCleanup := func() { _ = os.RemoveAll(versionDir) }

	zipPath := filepath.Join(versionDir, ".download.zip")
	if err := httpDownloadFile(ctx, cfg, dl, zipPath); err != nil {
		failCleanup()
		return err
	}
	defer func() { _ = os.Remove(zipPath) }()

	got, err := fileSHA256Hex(zipPath)
	if err != nil {
		failCleanup()
		return err
	}
	if !strings.EqualFold(got, wantSHA) {
		failCleanup()
		return fmt.Errorf("sha256 mismatch: want %s got %s", wantSHA, got)
	}

	if err := unzipSafe(versionDir, zipPath); err != nil {
		failCleanup()
		return fmt.Errorf("unzip: %w", err)
	}

	if err := os.WriteFile(filepath.Join(versionDir, "version.txt"), []byte(versionutil.Normalize(ver)+"\n"), 0644); err != nil {
		failCleanup()
		return fmt.Errorf("version.txt: %w", err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, FilePackageSHA256), []byte(strings.ToLower(wantSHA)+"\n"), 0644); err != nil {
		failCleanup()
		return fmt.Errorf("package sha file: %w", err)
	}

	reqPath := filepath.Join(versionDir, "requirements.txt")
	if _, err := os.Stat(reqPath); err != nil {
		if os.IsNotExist(err) {
			_ = os.Remove(filepath.Join(versionDir, MarkerPreparing))
			return nil
		}
		failCleanup()
		return err
	}

	pipCtx := ctx
	var cancel context.CancelFunc
	if cfg.PipInstallTimeout > 0 {
		pipCtx, cancel = context.WithTimeout(ctx, cfg.PipInstallTimeout)
		defer cancel()
	}
	py := EffectivePython(cfg.PythonExecutable)
	cmd := exec.CommandContext(pipCtx, py, "-m", "pip", "install", "-r", "requirements.txt")
	cmd.Dir = versionDir
	cmd.Env = os.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		_ = os.WriteFile(filepath.Join(versionDir, MarkerEnvFailed), []byte(msg), 0644)
		_ = os.Remove(filepath.Join(versionDir, MarkerPreparing))
		return fmt.Errorf("pip install: %w", err)
	}
	_ = os.Remove(filepath.Join(versionDir, MarkerPreparing))
	if log != nil {
		log.Info("sync: script env ready", "script_id", scriptID, "version", versionutil.Normalize(ver))
	}
	return nil
}

func httpDownloadFile(ctx context.Context, cfg *Config, dl *v1.DownloadSpec, destPath string) error {
	method := strings.ToUpper(strings.TrimSpace(dl.GetMethod()))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet {
		return fmt.Errorf("download: unsupported method %q", method)
	}
	urlStr := strings.TrimSpace(dl.GetUrl())
	if u, err := url.Parse(urlStr); err == nil && !u.IsAbs() && strings.TrimSpace(cfg.BaseURL) != "" {
		base, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/") + "/")
		if err == nil {
			urlStr = base.ResolveReference(u).String()
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, urlStr, nil)
	if err != nil {
		return fmt.Errorf("download request: %w", err)
	}
	for k, v := range dl.GetHeaders() {
		if strings.TrimSpace(k) == "" {
			continue
		}
		req.Header.Set(k, v)
	}
	if req.Header.Get("Authorization") == "" && cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	client := &http.Client{Timeout: cfg.HTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download http status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create zip file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("write zip: %w", err)
	}
	return nil
}

func fileSHA256Hex(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func unzipSafe(destDir, zipPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.Reader.File {
		tpath, err := safeZipEntryPath(destDir, f.Name)
		if err != nil {
			return err
		}
		if tpath == "" {
			continue
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(tpath, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(tpath), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		mode := f.Mode()
		if mode == 0 {
			mode = 0644
		}
		out, err := os.OpenFile(tpath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func safeZipEntryPath(dest, name string) (string, error) {
	name = strings.TrimSuffix(strings.TrimPrefix(filepath.ToSlash(name), "/"), "/")
	if name == "" {
		return "", nil
	}
	for _, p := range strings.Split(name, "/") {
		if p == ".." || p == "" {
			return "", fmt.Errorf("zip slip: %q", name)
		}
	}
	target := filepath.Join(dest, filepath.FromSlash(name))
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absDest, absTarget)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("zip slip: %q", name)
	}
	return target, nil
}
