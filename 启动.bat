@echo off
rem ============================================
rem  考公刷题 - 本地启动器（双击即可）
rem  关闭本窗口即停止服务
rem ============================================
chcp 65001 >nul
cd /d "%~dp0"

where go >nul 2>nul
if errorlevel 1 (
    echo [错误] 未检测到 Go，请先安装 Go 1.24+：https://go.dev/dl/
    pause
    exit /b 1
)

set "KAOGONG_ADDR=127.0.0.1:8080"
if exist "data\kaogong.db" goto run

echo.
echo  首次启动：请设置管理员密码（至少 8 位）
set /p "KAOGONG_ADMIN_PASSWORD=管理员密码: "
if "%KAOGONG_ADMIN_PASSWORD%"=="" (
    echo [错误] 密码不能为空
    pause
    exit /b 1
)

:run
echo.
echo  正在启动考公刷题服务...
echo.
echo  本机访问: http://localhost:8080
for /f "tokens=2 delims=:" %%a in ('ipconfig ^| findstr /c:"IPv4"') do echo  局域网访问: http://%%a:8080 ^(手机需连同一WiFi^)
echo.
echo  提示：关闭本窗口 = 停止服务
echo.

rem 5 秒后自动打开浏览器
start "" /min cmd /c "timeout /t 5 /nobreak >nul & start http://localhost:8080"

set "CGO_ENABLED=0"
go run ./cmd/kaogong
pause
