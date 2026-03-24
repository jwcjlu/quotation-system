<#
.SYNOPSIS
  批量设置 caichip-agent 环境变量（与 internal/agentapp/config.go 一致），并注册计划任务实现登录/开机启动与保活。

.DESCRIPTION
  - 从 agent.env（KEY=VALUE，# 注释）读取并写入环境变量（User 或 Machine）。
  - 注册计划任务运行 run-agent-watchdog.ps1，进程退出后自动重启。

.PARAMETER EnvFile
  环境变量文件路径，默认与脚本同目录下的 agent.env

.PARAMETER AgentExe
  agent.exe 绝对路径；若省略则使用 CAICHIP_AGENT_EXE 环境变量

.PARAMETER Scope
  User（默认，无需管理员）或 Machine（需管理员，全机生效）

.PARAMETER Trigger
  Logon：当前用户登录后启动（默认，一般无需管理员）
  Startup：系统启动时（可能需管理员；无用户登录时也可尝试运行，视账户与策略而定）

.EXAMPLE
  .\install-caichip-agent.ps1 -AgentExe "D:\caichip\agent.exe" -EnvFile "D:\caichip\scripts\windows\agent.env"
#>
[CmdletBinding()]
param(
  [string]$EnvFile = "",
  [string]$AgentExe = "",
  [ValidateSet("User", "Machine")]
  [string]$Scope = "User",
  [ValidateSet("Logon", "Startup")]
  [string]$Trigger = "Logon",
  [int]$RestartSec = 5,
  [switch]$Uninstall
)

$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$taskName = "CaichipAgentWatchdog"

if ($Uninstall) {
  Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction SilentlyContinue
  Write-Host "已移除计划任务: $taskName"
  exit 0
}

if ([string]::IsNullOrWhiteSpace($EnvFile)) {
  $EnvFile = Join-Path $here "agent.env"
}
if (-not (Test-Path -LiteralPath $EnvFile)) {
  Write-Error "找不到环境文件: $EnvFile （可先复制 agent.env.example 为 agent.env）"
}

$watchdog = Join-Path $here "run-agent-watchdog.ps1"
if (-not (Test-Path -LiteralPath $watchdog)) {
  Write-Error "找不到保活脚本: $watchdog"
}

if ([string]::IsNullOrWhiteSpace($AgentExe)) {
  $AgentExe = [Environment]::GetEnvironmentVariable("CAICHIP_AGENT_EXE", "User")
  if ([string]::IsNullOrWhiteSpace($AgentExe)) {
    $AgentExe = [Environment]::GetEnvironmentVariable("CAICHIP_AGENT_EXE", "Machine")
  }
}
if ([string]::IsNullOrWhiteSpace($AgentExe)) {
  Write-Error "请指定 -AgentExe 路径，或先设置用户环境变量 CAICHIP_AGENT_EXE"
}
$AgentExe = (Resolve-Path -LiteralPath $AgentExe).Path

# ---------- 解析 agent.env ----------
function Import-DotEnvFile([string]$path) {
  $vars = @{}
  Get-Content -LiteralPath $path -Encoding UTF8 | ForEach-Object {
    $line = $_.Trim()
    if ($line -match "^\s*#" -or $line -eq "") { return }
    $idx = $line.IndexOf("=")
    if ($idx -lt 1) { return }
    $name = $line.Substring(0, $idx).Trim()
    $val = $line.Substring($idx + 1).Trim()
    if (($val.StartsWith('"') -and $val.EndsWith('"')) -or ($val.StartsWith("'") -and $val.EndsWith("'"))) {
      $val = $val.Substring(1, $val.Length - 2)
    }
    if ($name -ne "") { $vars[$name] = $val }
  }
  return $vars
}

$knownKeys = @(
  "CAICHIP_API_KEY",
  "CAICHIP_BASE_URL",
  "AGENT_ID",
  "AGENT_TAGS",
  "AGENT_DATA_DIR",
  "AGENT_LONG_POLL_SEC",
  "AGENT_HTTP_TIMEOUT_SEC",
  "AGENT_SCRIPT_SYNC_SEC",
  "AGENT_MAX_PARALLEL",
  "AGENT_PIP_INSTALL_SEC",
  "CAICHIP_PYTHON",
  "AGENT_QUEUE",
  "CAICHIP_SKIP_PYTHON_CHECK",
  "CAICHIP_AUTO_INSTALL_PYTHON",
  "CAICHIP_AGENT_EXE"
)

$parsed = Import-DotEnvFile $EnvFile
foreach ($k in $parsed.Keys) {
  if ($k -notin $knownKeys) {
    Write-Warning "agent.env 中存在未识别键（已忽略）: $k"
  }
}

if ($Scope -eq "Machine") {
  $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)
  if (-not $isAdmin) {
    Write-Error "Scope=Machine 需要以管理员身份运行 PowerShell"
  }
}

if (-not $parsed.ContainsKey("CAICHIP_API_KEY") -or [string]::IsNullOrWhiteSpace($parsed["CAICHIP_API_KEY"])) {
  Write-Error "agent.env 中必须设置非空的 CAICHIP_API_KEY"
}

# 将 AgentExe 一并写入，便于日后排查与手动启动
$parsed["CAICHIP_AGENT_EXE"] = $AgentExe

foreach ($k in $knownKeys) {
  if (-not $parsed.ContainsKey($k)) { continue }
  $v = $parsed[$k]
  if ($null -eq $v) { $v = "" }
  [Environment]::SetEnvironmentVariable($k, $v, $Scope)
  Write-Host "已设置 ${Scope}: $k"
}

Write-Host ""
Write-Host "Env vars saved. New terminals / scheduled tasks will pick them up (re-login if needed)."

# ---------- 计划任务：保活 ----------
$arg = "-NoProfile -ExecutionPolicy Bypass -File `"$watchdog`" -AgentExe `"$AgentExe`" -RestartSec $RestartSec"
$action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument $arg

if ($Trigger -eq "Logon") {
  $tr = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
}
else {
  $tr = New-ScheduledTaskTrigger -AtStartup
}

$settings = New-ScheduledTaskSettingsSet `
  -AllowStartIfOnBatteries `
  -DontStopIfGoingOnBatteries `
  -StartWhenAvailable `
  -ExecutionTimeLimit ([TimeSpan]::Zero) `
  -RestartCount 3 `
  -RestartInterval (New-TimeSpan -Minutes 1)

$principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Limited

Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction SilentlyContinue
Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $tr -Settings $settings -Principal $principal | Out-Null

Write-Host ""
# Use ASCII-only + -f to avoid encoding / smart-quote issues on some Windows consoles
Write-Host ('Scheduled task registered: {0} trigger={1} (watchdog: run-agent-watchdog.ps1)' -f $taskName, $Trigger)
Write-Host ('Run test: Start-ScheduledTask -TaskName ''{0}''' -f $taskName)
