Param(
    [switch]$Quick
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Run-Step {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][string]$Command
    )
    Write-Host ""
    Write-Host "==> $Name" -ForegroundColor Cyan
    Write-Host "    $Command" -ForegroundColor DarkGray
    Invoke-Expression $Command
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "go command not found in PATH. Please install Go or open a shell with Go environment."
}

Run-Step -Name "HS Biz Acceptance" -Command "go test ./internal/biz/... -run Acceptance -v"
Run-Step -Name "HS Service Contract" -Command "go test ./internal/service/... -run HsResolveService -v"
Run-Step -Name "HS Observability" -Command "go test ./internal/biz/... -run Observability -v"

if (-not $Quick) {
    Run-Step -Name "Data Package Regression" -Command "go test ./internal/data/..."
    Run-Step -Name "Server Build" -Command "go build ./cmd/server/..."
}

Write-Host ""
Write-Host "HS resolve acceptance checks passed." -ForegroundColor Green
