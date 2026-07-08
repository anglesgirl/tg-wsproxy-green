package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

func main() {
	// 获取 exe 所在目录(兼容中文路径)
	exePath, err := os.Executable()
	if err != nil {
		messageBox("错误", fmt.Sprintf("无法获取程序路径: %v", err))
		os.Exit(1)
	}
	rootDir := filepath.Dir(exePath)

	proxyExe := filepath.Join(rootDir, "wsproxy", "TgWsProxy.exe")
	tgExe := filepath.Join(rootDir, "Telegram", "Telegram.exe")
	dataDir := filepath.Join(rootDir, "wsproxy", "TgWsProxy_data")
	markerFile := filepath.Join(rootDir, ".configured")

	// 检查文件是否存在
	if _, err := os.Stat(proxyExe); os.IsNotExist(err) {
		messageBox("错误", fmt.Sprintf("找不到代理程序:\n%s\n\n请确保 wsproxy\\TgWsProxy.exe 存在", proxyExe))
		os.Exit(1)
	}
	if _, err := os.Stat(tgExe); os.IsNotExist(err) {
		messageBox("错误", fmt.Sprintf("找不到 Telegram:\n%s\n\n请确保 Telegram\\Telegram.exe 存在", tgExe))
		os.Exit(1)
	}

	// 判断是否首次运行:检查标记文件或数据目录
	firstRun := !fileExists(markerFile) && !dirExists(dataDir)

	// 如果代理已在运行就不重复启动
	proxyAlreadyRunning := processExists("TgWsProxy.exe")

	if !proxyAlreadyRunning {
		if firstRun {
			// ===== 首次运行:显示代理窗口,让用户操作 =====
			messageBox("首次使用", "即将打开代理程序窗口。\n\n请在代理窗口中:\n1. 点击「连接」按钮\n2. 确认连接成功(显示绿色或已连接)\n\n连接成功后,点击本程序的「确定」按钮启动 Telegram。")

			// 启动代理(显示窗口,用户需要看到界面)
			proxyCmd := exec.Command(proxyExe)
			proxyCmd.SysProcAttr = &syscall.SysProcAttr{
				HideWindow: false,
			}
			if err := proxyCmd.Start(); err != nil {
				messageBox("错误", fmt.Sprintf("启动代理失败:\n%v", err))
				os.Exit(1)
			}

			// 等待用户在代理窗口操作完毕
			messageBox("等待确认", "请在代理窗口中点击「连接」按钮。\n\n连接成功后,点击「确定」启动 Telegram。")

			// 创建标记文件,以后不再弹窗
			os.WriteFile(markerFile, []byte("configured"), 0644)

			// 启动 Telegram
			startTelegram(tgExe)

			// 等待 Telegram 退出
			waitTelegramExit()

			// 清理代理
			killProcess("TgWsProxy.exe")
		} else {
			// ===== 后续运行:静默启动,自动运行 =====
			proxyCmd := exec.Command(proxyExe)
			proxyCmd.SysProcAttr = &syscall.SysProcAttr{
				HideWindow: false, // 不隐藏,让用户能看到状态
			}
			if err := proxyCmd.Start(); err != nil {
				messageBox("错误", fmt.Sprintf("启动代理失败:\n%v", err))
				os.Exit(1)
			}
			// 等待代理初始化
			time.Sleep(5 * time.Second)

			// 启动 Telegram
			startTelegram(tgExe)

			// 等待 Telegram 退出
			waitTelegramExit()

			// 清理代理
			killProcess("TgWsProxy.exe")
		}
	} else {
		// 代理已在运行,直接启动 TG
		startTelegram(tgExe)
		waitTelegramExit()
	}
	os.Exit(0)
}

// startTelegram 启动 Telegram
func startTelegram(tgExe string) {
	cmd := exec.Command(tgExe)
	cmd.Dir = filepath.Dir(tgExe)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: false,
	}
	cmd.Start()
}

// waitTelegramExit 等待 Telegram 退出
func waitTelegramExit() {
	for {
		time.Sleep(2 * time.Second)
		if !processExists("Telegram.exe") {
			break
		}
	}
}

// fileExists 检查文件是否存在
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// dirExists 检查目录是否存在
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

// processExists 检查进程是否存在(Windows)
func processExists(name string) bool {
	cmd := exec.Command("tasklist", "/fi", fmt.Sprintf("imagename eq %s", name), "/fo", "csv", "/nh")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	output := string(out)
	if len(output) < 2 {
		return false
	}
	return output[0] == '"'
}

// killProcess 结束进程(Windows)
func killProcess(name string) {
	cmd := exec.Command("taskkill", "/im", name, "/f")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Run()
}

// messageBox 弹出 Windows 消息框
func messageBox(title, text string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	mbox := user32.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(title)
	// MB_ICONINFORMATION = 0x40
	mbox.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x40)
}
