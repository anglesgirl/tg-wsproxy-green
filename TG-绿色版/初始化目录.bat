@echo off
chcp 65001 >nul 2>&1

REM 创建目录结构
echo 正在创建绿色版目录结构...
mkdir wsproxy 2>nul
mkdir Telegram 2>nul

echo.
echo ====================================
echo  目录已创建,请按提示放入文件
echo ====================================
echo.
echo [1] 把 tg-ws-proxy 的 exe 放到:
echo     %~dp0wsproxy\TgWsProxy.exe
echo.
echo     下载地址: https://github.com/Flowseal/tg-ws-proxy/releases/latest
echo     选 TgWsProxy-windows-x64-console.zip
echo     解压后重命名为 TgWsProxy.exe
echo.
echo [2] 把 Telegram Desktop 放到:
echo     %~dp0Telegram\Telegram.exe
echo.
echo     下载地址: https://desktop.telegram.org/
echo     选 Windows Portable 版本
echo.
echo [3] 放好后双击 启动Telegram.vbs 即可
echo.
pause
