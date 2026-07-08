package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func main() {
	exePath, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	rootDir := filepath.Dir(exePath)

	proxyExe := filepath.Join(rootDir, "wsproxy", "TgWsProxy.exe")
	tgExe := filepath.Join(rootDir, "Telegram", "Telegram.exe")

	if _, err := os.Stat(proxyExe); os.IsNotExist(err) {
		os.Exit(1)
	}
	if _, err := os.Stat(tgExe); os.IsNotExist(err) {
		os.Exit(1)
	}

	proxyAlreadyRunning := processExists("TgWsProxy.exe")

	if !proxyAlreadyRunning {
		// 启动代理,窗口正常显示
		proxyCmd := exec.Command(proxyExe)
		proxyCmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow: false,
		}
		proxyCmd.Start()

		// 等待代理窗口出现
		time.Sleep(3 * time.Second)
	}

	// 把代理窗口从托盘/最小化状态恢复显示出来
	restoreProxyWindow()

	// 等代理稳定
	time.Sleep(2 * time.Second)

	// 启动 Telegram
	cmd := exec.Command(tgExe)
	cmd.Dir = filepath.Dir(tgExe)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: false,
	}
	cmd.Start()

	// 等待 Telegram 退出
	for {
		time.Sleep(2 * time.Second)
		if !processExists("Telegram.exe") {
			break
		}
	}

	// 关闭代理
	if !proxyAlreadyRunning {
		killProcess("TgWsProxy.exe")
	}
	os.Exit(0)
}

// restoreProxyWindow 枚举所有窗口,找到代理窗口并显示出来
func restoreProxyWindow() {
	user32 := syscall.NewLazyDLL("user32.dll")
	enumWindows := user32.NewProc("EnumWindows")
	getWindowText := user32.NewProc("GetWindowTextW")
	showWindow := user32.NewProc("ShowWindow")
	setForegroundWindow := user32.NewProc("SetForegroundWindow")

	var foundHwnd uintptr

	// 回调:枚举每个顶层窗口,检查标题是否匹配代理程序
	cb := syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		var title [256]uint16
		ret, _, _ := getWindowText.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), 256)
		if ret > 0 {
			titleStr := syscall.UTF16ToString(title[:])
			lower := strings.ToLower(titleStr)
			// 匹配代理窗口标题
			if strings.Contains(lower, "proxy") || strings.Contains(lower, "tg ws") || strings.Contains(lower, "tgwsproxy") {
				foundHwnd = hwnd
				return 0 // 停止枚举
			}
		}
		return 1 // 继续
	})

	enumWindows.Call(cb, 0)

	if foundHwnd != 0 {
		// SW_RESTORE = 9:从最小化/最大化恢复
		showWindow.Call(foundHwnd, 9)
		// SW_SHOW = 5:显示窗口
		showWindow.Call(foundHwnd, 5)
		// 提到前台
		setForegroundWindow.Call(foundHwnd)
	}
}

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

func killProcess(name string) {
	cmd := exec.Command("taskkill", "/im", name, "/f")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Run()
}
