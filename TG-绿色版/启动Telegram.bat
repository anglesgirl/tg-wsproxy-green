@echo off
chcp 65001 >nul 2>&1
title TG 绿色版启动器

REM ============================================
REM  Telegram 绿色版启动器
REM  自动先启动 wsproxy,再启动 Telegram
REM  关闭 Telegram 后自动清理代理进程
REM ============================================

REM ----- 路径配置(按实际目录结构修改) -----
set "ROOT=%~dp0"
set "PROXY_EXE=%ROOT%wsproxy\TgWsProxy.exe"
set "PROXY_PORT=1080"
set "TG_EXE=%ROOT%Telegram\Telegram.exe"

REM ----- 检查文件是否存在 -----
if not exist "%PROXY_EXE%" (
    echo [错误] 找不到代理程序: %PROXY_EXE%
    echo 请把 tg-ws-proxy 的程序放到 wsproxy 文件夹
    pause
    exit /b 1
)
if not exist "%TG_EXE%" (
    echo [错误] 找不到 Telegram: %TG_EXE%
    echo 请把 Telegram Desktop 放到 Telegram 文件夹
    pause
    exit /b 1
)

REM ----- 检查端口是否已被占用(代理可能已在运行) -----
netstat -ano | findstr ":%PROXY_PORT% " | findstr "LISTENING" >nul 2>&1
if %errorlevel%==0 (
    echo [信息] 代理端口 %PROXY_PORT% 已在监听,跳过启动代理
    goto :start_tg
)

REM ----- 启动代理(静默后台) -----
echo [步骤 1/2] 正在启动代理...
start "" /b "%PROXY_EXE%"

REM ----- 等待代理端口就绪 -----
set /a WAIT=0
:wait_proxy
timeout /t 1 /nobreak >nul
set /a WAIT+=1
netstat -ano | findstr ":%PROXY_PORT% " | findstr "LISTENING" >nul 2>&1
if %errorlevel%==0 (
    echo [信息] 代理已就绪 (耗时 %WAIT% 秒)
    goto :start_tg
)
if %WAIT% geq 15 (
    echo [警告] 代理 15 秒内未就绪,直接启动 Telegram
    goto :start_tg
)
goto :wait_proxy

:start_tg
echo [步骤 2/2] 正在启动 Telegram...
start "" "%TG_EXE%"
echo [完成] 已启动。关闭 Telegram 后本窗口会自动清理代理。
echo.

REM ----- 等待 Telegram 退出 -----
:wait_tg
tasklist /fi "imagename eq Telegram.exe" 2>nul | find /i "Telegram.exe" >nul
if %errorlevel%==0 (
    timeout /t 2 /nobreak >nul
    goto :wait_tg
)

REM ----- Telegram 已关闭,清理代理进程 -----
echo [清理] Telegram 已退出,正在关闭代理...
taskkill /im TgWsProxy.exe /f >nul 2>&1
echo [完成] 代理已关闭。
timeout /t 1 /nobreak >nul
