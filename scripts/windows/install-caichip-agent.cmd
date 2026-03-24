@echo off
REM 避免「禁止运行脚本」：用 Bypass 调用同目录下的 install-caichip-agent.ps1
set "SCRIPT=%~dp0install-caichip-agent.ps1"
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT%" %*
