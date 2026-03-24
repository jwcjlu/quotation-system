<#
.SYNOPSIS
  保活：caichip-agent 退出后等待数秒自动重启（用于计划任务开机/登录启动）。
#>
param(
  [Parameter(Mandatory = $true)]
  [string]$AgentExe,
  [int]$RestartSec = 5,
  [string]$LogFile = ""
)

$ErrorActionPreference = "Stop"
$AgentExe = (Resolve-Path -LiteralPath $AgentExe).Path
$workDir = Split-Path -LiteralPath $AgentExe
if ([string]::IsNullOrWhiteSpace($LogFile)) {
  $LogFile = Join-Path $workDir "agent-watchdog.log"
}

function Write-WdLog([string]$msg) {
  $line = "[{0}] {1}" -f (Get-Date -Format "yyyy-MM-dd HH:mm:ss"), $msg
  Add-Content -LiteralPath $LogFile -Encoding UTF8 -Value $line
  Write-Host $line
}

Write-WdLog "watchdog start AgentExe=$AgentExe RestartSec=$RestartSec"

while ($true) {
  Write-WdLog "starting agent process..."
  try {
    $p = Start-Process -FilePath $AgentExe -WorkingDirectory $workDir -NoNewWindow -Wait -PassThru
    $code = $p.ExitCode
  }
  catch {
    Write-WdLog "Start-Process failed: $($_.Exception.Message)"
    $code = -1
  }
  Write-WdLog "agent exited exitCode=$code, sleep ${RestartSec}s then restart"
  Start-Sleep -Seconds $RestartSec
}
