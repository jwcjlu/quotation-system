# Windows：环境变量 + 开机/登录启动 + 保活

## 文件

| 文件 | 说明 |
|------|------|
| `agent.env.example` | 与 `internal/agentapp/config.go` 对应的环境变量模板 |
| `agent.env` | 你本地配置（复制 example 后修改，**勿提交密钥**） |
| `run-agent-watchdog.ps1` | 保活：`agent.exe` 退出后等待几秒再拉起 |
| `install-caichip-agent.ps1` | 从 `agent.env` 批量写入环境变量，并注册计划任务 |
| `install-caichip-agent.cmd` | 同上（绕过执行策略，参数原样传给 `.ps1`） |

## 编码（若报「字符串缺少终止符」或乱码）

请使用 **UTF-8（建议带 BOM）** 保存 `install-caichip-agent.ps1`。仓库内该文件已用 UTF-8 BOM 保存，便于 Windows PowerShell 5.1 正确解析中文；若你手动编辑，请用编辑器保存为 UTF-8 with BOM，或从本仓库重新复制脚本。

## 执行策略（若提示「禁止运行脚本」）

任选其一：

- **不改策略**：双击或命令行运行 **`install-caichip-agent.cmd`**（内部已 `-ExecutionPolicy Bypass`），或：
  ```bat
  powershell -NoProfile -ExecutionPolicy Bypass -File ".\install-caichip-agent.ps1" -AgentExe "D:\path\to\agent.exe"
  ```
- **当前会话**：`Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass`
- **当前用户长期**：`Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned`

## 使用步骤

1. 编译出 `agent.exe`（例如 `go build -o agent.exe ./cmd/agent`）。
2. 复制 `agent.env.example` 为 `agent.env`，填写 `CAICHIP_API_KEY` 等。
3. **管理员 PowerShell**（若需 `Scope=Machine`）或普通用户（默认 `Scope=User`）执行：

```powershell
cd D:\workspace\caichip\scripts\windows
.\install-caichip-agent.cmd -AgentExe "D:\path\to\agent.exe" -EnvFile ".\agent.env"
```

或（已允许脚本时）直接 `.\install-caichip-agent.ps1 ...`。

4. 立即试跑计划任务：

```powershell
Start-ScheduledTask -TaskName 'CaichipAgentWatchdog'
```

5. 卸载计划任务（不删环境变量）：

```powershell
.\install-caichip-agent.ps1 -Uninstall
```

## 参数说明

- `-AgentExe`：`agent.exe` 绝对路径。
- `-EnvFile`：默认当前目录 `agent.env`。
- `-Scope User|Machine`：环境变量写入「当前用户」或「本机」（Machine 需管理员）。
- `-Trigger Logon|Startup`：登录后启动（默认）或系统启动时启动（Startup 视账户策略可能需管理员）。
- `-RestartSec`：保活脚本中进程退出后的等待秒数（默认 5）。
- `-Uninstall`：仅删除计划任务 `CaichipAgentWatchdog`。

保活日志默认写在 `agent.exe` 同目录：`agent-watchdog.log`。

## 环境变量一览（与 `LoadConfig` 一致）

- `CAICHIP_API_KEY`（必填）
- `CAICHIP_BASE_URL`（默认 `http://127.0.0.1:18080`）
- `AGENT_ID`、`AGENT_QUEUE`、`AGENT_TAGS`
- `AGENT_DATA_DIR`、`CAICHIP_PYTHON`
- `AGENT_LONG_POLL_SEC`、`AGENT_HTTP_TIMEOUT_SEC`、`AGENT_SCRIPT_SYNC_SEC`
- `AGENT_MAX_PARALLEL`、`AGENT_PIP_INSTALL_SEC`
- `CAICHIP_SKIP_PYTHON_CHECK`、`CAICHIP_AUTO_INSTALL_PYTHON`

安装脚本会额外写入 `CAICHIP_AGENT_EXE`（指向本次指定的 `agent.exe`）。
