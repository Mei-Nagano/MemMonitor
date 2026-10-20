@echo off
setlocal
cd /d "%~dp0"

where go >nul 2>nul
if errorlevel 1 (
    echo [ERROR] 没找到 go 命令，请先安装 Go: https://go.dev/dl/
    pause
    exit /b 1
)

echo 正在编译 memmon.exe ...
go build -trimpath -ldflags "-s -w -H windowsgui" -o memmon.exe .
if errorlevel 1 (
    echo [ERROR] 编译失败
    pause
    exit /b 1
)

echo 完成: %cd%\memmon.exe
pause
