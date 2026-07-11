package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	ProxyPort   = 1443
	ProxyHost   = "127.0.0.1"
	FixedSecret = "ee8a1b8e0c2d4f6a9b3e7c5d1f2a4b6c"
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
		msgBox("错误", "未找到 wsproxy\\TgWsProxy.exe")
		os.Exit(1)
	}
	if _, err := os.Stat(tgExe); os.IsNotExist(err) {
		msgBox("错误", "未找到 Telegram\\Telegram.exe")
		os.Exit(1)
	}

	// 1. 预创建代理配置（固定 secret）
	ensureProxyConfig()

	// 2. 启动代理
	proxyAlreadyRunning := processExists("TgWsProxy.exe")
	if !proxyAlreadyRunning {
		proxyCmd := exec.Command(proxyExe)
		proxyCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
		proxyCmd.Start()
	}

	// 3. 等待代理端口就绪
	waitForPort(ProxyHost, ProxyPort, 15*time.Second)

	// 4. 首次运行：打开 tg://proxy 配置代理
	markerFile := filepath.Join(rootDir, ".proxy_configured")
	savedSecret, _ := os.ReadFile(markerFile)
	savedSecretStr := strings.TrimSpace(string(savedSecret))

	if savedSecretStr != FixedSecret {
		// 首次运行：打开 tg://proxy 链接
		// 先启动 Telegram，再发 tg://proxy 链接让它配置代理
		if !processExists("Telegram.exe") {
			cmd := exec.Command(tgExe)
			cmd.Dir = filepath.Dir(tgExe)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
			cmd.Start()
			time.Sleep(3 * time.Second)
		}
		openURL(fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=%s", ProxyHost, ProxyPort, FixedSecret))
		os.WriteFile(markerFile, []byte(FixedSecret), 0644)
	} else {
		// 后续运行：直接启动 Telegram
		if !processExists("Telegram.exe") {
			cmd := exec.Command(tgExe)
			cmd.Dir = filepath.Dir(tgExe)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
			cmd.Start()
		}
	}

	// 5. 等待 Telegram 退出
	for {
		time.Sleep(2 * time.Second)
		if !processExists("Telegram.exe") {
			break
		}
	}

	// 6. 关闭代理
	if !proxyAlreadyRunning {
		killProcess("TgWsProxy.exe")
	}
	os.Exit(0)
}

func ensureProxyConfig() {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return
	}
	configDir := filepath.Join(appData, "TgWsProxy")
	os.MkdirAll(configDir, 0755)
	configPath := filepath.Join(configDir, "config.json")

	if data, err := os.ReadFile(configPath); err == nil {
		var config map[string]interface{}
		if json.Unmarshal(data, &config) == nil {
			if s, ok := config["secret"].(string); ok && s != "" {
				return
			}
		}
	}

	config := map[string]interface{}{
		"port":             ProxyPort,
		"host":             ProxyHost,
		"secret":           FixedSecret,
		"fallback_cfproxy": true,
	}
	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, data, 0644)
}

func waitForPort(host string, port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 1*time.Second)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func openURL(url string) {
	shell32 := syscall.NewLazyDLL("shell32.dll")
	shellExecute := shell32.NewProc("ShellExecuteW")
	openStr, _ := syscall.UTF16PtrFromString("open")
	urlStr, _ := syscall.UTF16PtrFromString(url)
	shellExecute.Call(0, uintptr(unsafe.Pointer(openStr)),
		uintptr(unsafe.Pointer(urlStr)), 0, 0, 1)
}

func msgBox(title, text string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	textPtr, _ := syscall.UTF16PtrFromString(text)
	messageBox.Call(0, uintptr(unsafe.Pointer(textPtr)),
		uintptr(unsafe.Pointer(titlePtr)), 0x40)
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
