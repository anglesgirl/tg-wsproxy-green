package main

import (
	"fmt"
	"net"
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
	proxyPort := "1080"

	// 检查文件是否存在
	if _, err := os.Stat(proxyExe); os.IsNotExist(err) {
		messageBox("错误", fmt.Sprintf("找不到代理程序:\n%s\n\n请确保 wsproxy\\TgWsProxy.exe 存在", proxyExe))
		os.Exit(1)
	}
	if _, err := os.Stat(tgExe); os.IsNotExist(err) {
		messageBox("错误", fmt.Sprintf("找不到 Telegram:\n%s\n\n请确保 Telegram\\Telegram.exe 存在", tgExe))
		os.Exit(1)
	}

	// 检查代理端口是否已在监听
	if isPortListening(proxyPort) {
		startTelegram(tgExe)
		waitAndCleanup()
		return
	}

	// 启动代理(静默后台,不弹窗)
	proxyCmd := exec.Command(proxyExe)
	proxyCmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
	if err := proxyCmd.Start(); err != nil {
		messageBox("错误", fmt.Sprintf("启动代理失败:\n%v", err))
		os.Exit(1)
	}

	// 等待代理端口就绪(最多 15 秒)
	ready := false
	for i := 0; i < 15; i++ {
		time.Sleep(time.Second)
		if isPortListening(proxyPort) {
			ready = true
			break
		}
	}

	if !ready {
		messageBox("警告", "代理 15 秒内未就绪\n将直接启动 Telegram(可能无法连接)")
	}

	// 启动 Telegram
	startTelegram(tgExe)

	// 等待 Telegram 退出,然后清理代理
	waitAndCleanup()
}

// isPortListening 检查本地端口是否在监听
func isPortListening(port string) bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
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

// waitAndCleanup 等待 Telegram 退出后关闭代理
func waitAndCleanup() {
	for {
		time.Sleep(2 * time.Second)
		if !processExists("Telegram.exe") {
			break
		}
	}
	// Telegram 已关闭,清理代理进程
	killProcess("TgWsProxy.exe")
	os.Exit(0)
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
	// tasklist 找不到时输出 "信息: 没有运行的任务..."
	// 找到时输出进程名开头的行
	if len(output) < 2 {
		return false
	}
	// CSV 输出找到进程会有引号开头
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
	// MB_ICONERROR = 0x10, MB_OK = 0x0
	mbox.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x10)
}
