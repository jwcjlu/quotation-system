package agentapp

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// VerifyPythonAndPip 启动前检查：解释器可执行且 `python -m pip` 可用。
// 若 Config.AutoInstallPython 为 true，会尝试自动安装（见 python_install.go）。
func VerifyPythonAndPip(cfg *Config) error {
	if cfg.SkipPythonCheck {
		return nil
	}

	py, err := findWorkingPython(cfg)
	if err != nil {
		if cfg.AutoInstallPython {
			if ierr := tryInstallPythonAndPip(cfg); ierr != nil {
				return fmt.Errorf("%w\n自动安装失败: %v\n\n%s", err, ierr, manualInstallHints())
			}
			py, err = findWorkingPython(cfg)
			if err != nil {
				return fmt.Errorf("自动安装后仍无法找到 Python: %w（若刚安装完 Python，请关闭终端后重开或重启 agent 以刷新 PATH）\n\n%s", err, manualInstallHints())
			}
		} else {
			return fmt.Errorf("%w\n\n%s", err, manualInstallHints())
		}
	}

	if err := checkPip(py); err != nil {
		if cfg.AutoInstallPython {
			_ = tryEnsurePip(py)
			if err2 := checkPip(py); err2 == nil {
				return nil
			}
			if ierr := tryInstallPythonAndPip(cfg); ierr == nil {
				py2, err3 := findWorkingPython(cfg)
				if err3 == nil && checkPip(py2) == nil {
					return nil
				}
			}
			return fmt.Errorf("pip 不可用（已尝试 ensurepip / 自动安装）: %v\n\n%s", err, manualInstallHints())
		}
		return fmt.Errorf("pip 不可用: %v\n\n提示: 安装 Python 时勾选 pip；默认会自动尝试修复，若需关闭自动安装请设置 CAICHIP_AUTO_INSTALL_PYTHON=0。\n\n%s", err, manualInstallHints())
	}
	return nil
}

func pythonCandidates(cfg *Config) []string {
	if s := strings.TrimSpace(cfg.PythonExecutable); s != "" {
		return []string{s}
	}
	if runtime.GOOS == "windows" {
		return []string{"python", "python3"}
	}
	return []string{"python3", "python"}
}

func findWorkingPython(cfg *Config) (string, error) {
	var lastErr error
	for _, name := range pythonCandidates(cfg) {
		path, err := exec.LookPath(name)
		if err != nil {
			lastErr = err
			continue
		}
		if err := runPythonSmoke(path); err != nil {
			lastErr = err
			continue
		}
		return path, nil
	}
	// 无 PATH 时（例如刚静默安装）：扫描 Windows 常见安装路径
	if runtime.GOOS == "windows" {
		for _, p := range discoverWindowsPythonExes() {
			if err := runPythonSmoke(p); err != nil {
				lastErr = err
				continue
			}
			return p, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("未找到 Python 解释器")
	}
	return "", fmt.Errorf("无法找到可用的 Python（已尝试: %v）: %w", pythonCandidates(cfg), lastErr)
}

// discoverWindowsPythonExes 在 PATH 未包含 python 时仍能找到本机已安装的 python.exe（如 winget/官方安装包用户级目录）。
func discoverWindowsPythonExes() []string {
	var out []string
	seen := map[string]bool{}
	add := func(p string) {
		p = filepath.Clean(p)
		if p == "" || seen[p] {
			return
		}
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	globAdd := func(pattern string) {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			add(m)
		}
	}
	if la := os.Getenv("LocalAppData"); la != "" {
		globAdd(filepath.Join(la, "Programs", "Python", "Python*", "python.exe"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		globAdd(filepath.Join(home, "AppData", "Local", "Programs", "Python", "Python*", "python.exe"))
	}
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		globAdd(filepath.Join(pf, "Python*", "python.exe"))
	}
	if pf86 := os.Getenv("ProgramFiles(x86)"); pf86 != "" {
		globAdd(filepath.Join(pf86, "Python*", "python.exe"))
	}
	return out
}

// firstWorkingWindowsPython 用于自动安装后探测：PATH 可能尚未刷新。
func firstWorkingWindowsPython() string {
	for _, p := range discoverWindowsPythonExes() {
		if runPythonSmoke(p) == nil {
			return p
		}
	}
	return ""
}

func runPythonSmoke(py string) error {
	cmd := exec.Command(py, "-c", "import sys; print(sys.version)")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("%q: %w: %s", py, err, strings.TrimSpace(stderr.String()))
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		return fmt.Errorf("%q: 无版本输出", py)
	}
	return nil
}

func checkPip(py string) error {
	cmd := exec.Command(py, "-m", "pip", "--version")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		return fmt.Errorf("pip 无输出")
	}
	return nil
}

func tryEnsurePip(py string) error {
	cmd := exec.Command(py, "-m", "ensurepip", "--upgrade")
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	return cmd.Run()
}

func manualInstallHints() string {
	switch runtime.GOOS {
	case "windows":
		return strings.TrimSpace(`
Windows:
  - 默认会自动尝试安装 Python（winget 或从 python.org 下载官方安装包静默安装）。
  - 若不想自动安装，请设置 CAICHIP_AUTO_INSTALL_PYTHON=0。
  - 手动安装: https://www.python.org/downloads/ ，勾选「Add Python to PATH」与 pip。`)
	case "darwin":
		return strings.TrimSpace(`
macOS:
  - 默认会尝试 brew install python（需已安装 Homebrew）。
  - 关闭自动安装: CAICHIP_AUTO_INSTALL_PYTHON=0`)
	default:
		return strings.TrimSpace(`
Linux (Debian/Ubuntu 示例):
  - 默认在 **root** 下会尝试 apt 安装 python3 / python3-pip；非 root 请 sudo apt-get install -y python3 python3-pip
  - 关闭自动安装: CAICHIP_AUTO_INSTALL_PYTHON=0
  - 其他发行版请用对应包管理器安装 python3 与 pip。`)
	}
}
