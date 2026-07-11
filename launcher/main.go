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
		msgBox("错误", "未找到 wsproxy\\TgWsProxy.exe\n请先下载 TgWsProxy.exe 放入 wsproxy 目录。")
		os.Exit(1)
	}

	// 1. 预创建代理配置文件（写入固定 secret）
	ensureProxyConfig()

	// 2. 启动代理（静默在托盘运行，不弹窗口）
	proxyAlreadyRunning := processExists("TgWsProxy.exe")
	if !proxyAlreadyRunning {
		proxyCmd := exec.Command(proxyExe)
		proxyCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
		proxyCmd.Start()
	}

	// 3. 等待代理端口就绪（最多等 15 秒）
	portReady := waitForPort(ProxyHost, ProxyPort, 15*time.Second)
	if !portReady {
		msgBox("提示", "代理启动较慢，正在尝试启动 Telegram…")
	}

	// 4. 读取实际 secret（代理可能用了自己的 secret）
	secret := getProxySecret()
	if secret == "" {
		secret = FixedSecret
	}

	// 5. 判断是否需要配置 Telegram 代理
	markerFile := filepath.Join(rootDir, ".proxy_configured")
	savedSecret, _ := os.ReadFile(markerFile)
	savedSecretStr := strings.TrimSpace(string(savedSecret))

	if savedSecretStr != secret {
		// 首次运行或 secret 变了：用 tg://proxy URL 自动配置
		// 先确保 Telegram 没在运行
		if processExists("Telegram.exe") {
			killProcess("Telegram.exe")
			time.Sleep(2 * time.Second)
		}

		// 打开 tg://proxy 深度链接，Telegram 会弹出"启用代理"对话框
		openURL(fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=%s", ProxyHost, ProxyPort, secret))

		// 等待 Telegram 启动
		time.Sleep(5 * time.Second)

		// 保存当前 secret
		os.WriteFile(markerFile, []byte(secret), 0644)
	} else {
		// 后续运行：直接启动 Telegram（代理已配好）
		if !processExists("Telegram.exe") {
			cmd := exec.Command(tgExe)
			cmd.Dir = filepath.Dir(tgExe)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
			cmd.Start()
		}
	}

	// 6. 等待 Telegram 退出
	for {
		time.Sleep(2 * time.Second)
		if !processExists("Telegram.exe") {
			break
		}
	}

	// 7. 关闭代理
	if !proxyAlreadyRunning {
		killProcess("TgWsProxy.exe")
	}
	os.Exit(0)
}

// ensureProxyConfig 预创建 TgWsProxy 的 config.json，写入固定 secret。
// 配置文件位置：%APPDATA%/TgWsProxy/config.json
func ensureProxyConfig() {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return
	}

	configDir := filepath.Join(appData, "TgWsProxy")
	os.MkdirAll(configDir, 0755)

	configPath := filepath.Join(configDir, "config.json")

	// 如果配置已存在且包含 secret，不覆盖
	if data, err := os.ReadFile(configPath); err == nil {
		var config map[string]interface{}
		if json.Unmarshal(data, &config) == nil {
			if s, ok := config["secret"].(string); ok && s != "" {
				return
			}
		}
	}

	// 创建配置
	config := map[string]interface{}{
		"port":             ProxyPort,
		"host":             ProxyHost,
		"secret":           FixedSecret,
		"fallback_cfproxy": true,
	}

	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, data, 0644)
}

// getProxySecret 从 TgWsProxy 的配置文件中读取实际 secret
func getProxySecret() string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return ""
	}

	configPath := filepath.Join(appData, "TgWsProxy", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	var config map[string]interface{}
	if json.Unmarshal(data, &config) != nil {
		return ""
	}

	if secret, ok := config["secret"].(string); ok && secret != "" {
		return secret
	}
	return ""
}

// waitForPort 轮询等待端口就绪
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

// openURL 用系统默认程序打开 URL（支持 tg:// 协议）
func openURL(url string) {
	shell32 := syscall.NewLazyDLL("shell32.dll")
	shellExecute := shell32.NewProc("ShellExecuteW")
	openStr, _ := syscall.UTF16PtrFromString("open")
	urlStr, _ := syscall.UTF16PtrFromString(url)
	shellExecute.Call(0, uintptr(unsafe.Pointer(openStr)),
		uintptr(unsafe.Pointer(urlStr)), 0, 0, 1)
}

// msgBox 显示一个 Windows 消息框
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
