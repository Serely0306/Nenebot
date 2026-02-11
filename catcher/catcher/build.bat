@echo off
REM LunaBot Catcher 编译脚本 (Windows)

echo ========================================
echo  LunaBot Catcher 编译脚本
echo ========================================
echo.

REM 检查 Go 是否安装
where go >nul 2>nul
if %ERRORLEVEL% neq 0 (
    echo 错误: 未找到 Go。请先安装 Go: https://golang.org/dl/
    pause
    exit /b 1
)

echo 1. 下载依赖...
go mod tidy
if %ERRORLEVEL% neq 0 (
    echo 下载依赖失败!
    pause
    exit /b 1
)

echo.
echo 2. 编译 Windows 版本...
go build -o LunaBotCatcher-windows-amd64.exe ./cmd/catcher
if %ERRORLEVEL% neq 0 (
    echo 编译 Windows 版本失败!
    pause
    exit /b 1
)
echo    - LunaBotCatcher-windows-amd64.exe

echo.
echo 3. 交叉编译 Android ARM64 版本...
set GOOS=android
set GOARCH=arm64
set CGO_ENABLED=0
go build -ldflags="-s -w" -o LunaBotCatcher-android-arm64 ./cmd/catcher
if %ERRORLEVEL% neq 0 (
    echo 编译 Android 版本失败!
    pause
    exit /b 1
)
echo    - LunaBotCatcher-android-arm64

echo.
echo 4. 交叉编译 Linux AMD64 版本...
set GOOS=linux
set GOARCH=amd64
set CGO_ENABLED=0
go build -ldflags="-s -w" -o LunaBotCatcher-linux-amd64 ./cmd/catcher
if %ERRORLEVEL% neq 0 (
    echo 编译 Linux 版本失败!
)
echo    - LunaBotCatcher-linux-amd64

echo.
echo ========================================
echo  编译完成!
echo ========================================
echo.
echo Android 使用方法:
echo   adb push LunaBotCatcher-android-arm64 /data/local/tmp/lunabot-catcher/
echo   adb push config.yaml /data/local/tmp/lunabot-catcher/
echo   adb shell chmod 755 /data/local/tmp/lunabot-catcher/LunaBotCatcher-android-arm64
echo.

pause
