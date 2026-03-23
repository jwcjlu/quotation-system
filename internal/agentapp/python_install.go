package agentapp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// tryInstallPythonAndPip 在支持的平台尝试安装 Python 与 pip（默认开启，见 Config.AutoInstallPython）。
func tryInstallPythonAndPip(cfg *Config) error {
	switch runtime.GOOS {
	case "windows":
		return installPythonWindows()
	case "linux":
		return installPythonLinux()
	case "darwin":
		return installPythonDarwin()
	default:
		return fmt.Errorf("当前系统 (%s) 不支持自动安装 Python", runtime.GOOS)
	}
}

func findWingetPath() string {
	if p, err := exec.LookPath("winget"); err == nil {
		return p
	}
	if la := os.Getenv("LocalAppData"); la != "" {
		p := filepath.Join(la, "Microsoft", "WindowsApps", "winget.exe")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func installPythonWindows() error {
	if w := findWingetPath(); w != "" {
		err := runWingetPython(w)
		time.Sleep(2 * time.Second)
		if err == nil {
			return nil
		}
		// 可能已安装但 PATH 未含 python，或 winget 报错但本机已有 Python
		if firstWorkingWindowsPython() != "" {
			return nil
		}
	}
	return installPythonWindowsDownloadExe()
}

func runWingetPython(winget string) error {
	cmd := exec.Command(winget, "install", "-e",
		"--id", "Python.Python.3.12",
		"--accept-package-agreements",
		"--accept-source-agreements",
	)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	combined := strings.ToLower(out.String() + errBuf.String())
	if err != nil && (strings.Contains(combined, "already installed") ||
		strings.Contains(combined, "no applicable upgrade") ||
		strings.Contains(combined, "no newer package")) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("winget: %w\n%s\n%s", err, out.String(), errBuf.String())
	}
	return nil
}

func pythonWindowsInstallerURL() string {
	switch runtime.GOARCH {
	case "386":
		return "https://www.python.org/ftp/python/3.12.8/python-3.12.8.exe"
	case "arm64":
		return "https://www.python.org/ftp/python/3.12.8/python-3.12.8-arm64.exe"
	default:
		return "https://www.python.org/ftp/python/3.12.8/python-3.12.8-amd64.exe"
	}
}

func installPythonWindowsDownloadExe() error {
	url := pythonWindowsInstallerURL()
	client := &http.Client{Timeout: 20 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "caichip-agent/1.0 (python bootstrap)")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载官方 Python 安装包失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载官方 Python 安装包: HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "caichip-python-*.exe")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// 用户级静默安装，写入 PATH 与 pip（安装后当前进程 PATH 可能仍无 python，由 discoverWindowsPythonExes 探测）
	cmd := exec.Command(tmpPath,
		"/quiet",
		"InstallAllUsers=0",
		"PrependPath=1",
		"Include_pip=1",
		"Include_test=0",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("静默安装 Python 失败: %w", err)
	}
	time.Sleep(5 * time.Second)
	return nil
}

func installPythonLinux() error {
	out, err := exec.Command("id", "-u").Output()
	if err != nil {
		return fmt.Errorf("无法检测当前用户: %w", err)
	}
	if strings.TrimSpace(string(out)) != "0" {
		return fmt.Errorf("Linux 下自动安装需要 root（或使用 apt 手动安装 python3 python3-pip）")
	}
	apt := func(args ...string) error {
		c := exec.Command("apt-get", args...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	}
	if err := apt("update"); err != nil {
		return fmt.Errorf("apt-get update: %w", err)
	}
	if err := apt("install", "-y", "python3", "python3-pip"); err != nil {
		return fmt.Errorf("apt-get install python3 python3-pip: %w", err)
	}
	return nil
}

func installPythonDarwin() error {
	if _, err := exec.LookPath("brew"); err != nil {
		return fmt.Errorf("未找到 brew，请先安装 Homebrew 或手动安装 Python: %w", err)
	}
	cmd := exec.Command("brew", "install", "python")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("brew install python: %w", err)
	}
	return nil
}
