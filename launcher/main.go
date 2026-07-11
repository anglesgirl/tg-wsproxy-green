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

	if _, err := os.Stat(proxyExe); os.IsNotExist(err) {
		msgBox("错误", "未找到 wsproxy\\TgWsProxy.exe\n请先下载 TgWsProxy.exe 放入 wsproxy 目录。")
		os.Exit(1)
	}

	// 1. 预创建代理配置（固定 secret，确保不变）
	ensureProxyConfig()

	// 2. 启动代理
	proxyAlreadyRunning := processExists("TgWsProxy.exe")
	if !proxyAlreadyRunning {
		proxyCmd := exec.Command(proxyExe)
		proxyCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
		proxyCmd.Start()
	}

	// 3. 等待代理端口就绪
	portReady := waitForPort(ProxyHost, ProxyPort, 15*time.Second)
	if !portReady {
		msgBox("提示", "代理启动较慢，正在尝试启动 Telegram…")
	}

	// 4. 判断是否需要配置 Telegram 代理
	markerFile := filepath.Join(rootDir, ".proxy_configured")
	savedSecret, _ := os.ReadFile(markerFile)
	savedSecretStr := strings.TrimSpace(string(savedSecret))

	if savedSecretStr != FixedSecret {
		// 首次运行或 secret 变了：打开 tg://proxy 链接
		// Windows 会自动通过 tg:// 协议关联找到 Telegram 并启动
		// Telegram 弹出"启用代理"对话框，用户点"启用"即可
		if processExists("Telegram.exe") {
			killProcess("Telegram.exe")
			time.Sleep(2 * time.Second)
		}
		openURL(fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=%s", ProxyHost, ProxyPort, FixedSecret))
		time.Sleep(5 * time.Second)
		os.WriteFile(markerFile, []byte(FixedSecret), 0644)
	} else {
		// 后续运行：直接启动 Telegram
		// 代理已配好，Telegram 自动通过代理连接
		if !processExists("Telegram.exe") {
			tgPath := findTelegram()
			if tgPath != "" {
				cmd := exec.Command(tgPath)
				cmd.Dir = filepath.Dir(tgPath)
				cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
				cmd.Start()
			} else {
				// 找不到 Telegram，用 tg://proxy 触发系统关联
				openURL(fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=%s", ProxyHost, ProxyPort, FixedSecret))
			}
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

// findTelegram 自动查找 Telegram.exe，不需要用户配置：
// 1. 同级 Telegram\Telegram.exe
// 2. 注册表 tg:// 协议关联（和 Windows 点击 tg:// 链接用同样的方式）
// 3. %APPDATA%\Telegram Desktop
// 4. %LOCALAPPDATA%\Telegram Desktop
func findTelegram() string {
	exePath, _ := os.Executable()
	rootDir := filepath.Dir(exePath)

	// 1. 同级目录
	localPath := filepath.Join(rootDir, "Telegram", "Telegram.exe")
	if fileExists(localPath) {
		return localPath
	}

	// 2. 注册表 tg:// 协议关联
	// HKCU\Software\Classes\tg\shell\open\command
	regPath := readRegistry(`SOFTWARE\Classes\tg\shell\open\command`, "")
	if regPath != "" {
		// 格式通常是: "C:\...\Telegram.exe" "%1" 或 C:\...\Telegram.exe %1
		path := extractExePath(regPath)
		if path != "" && fileExists(path) {
			return path
		}
	}
	// 也试 HKLM
	regPath = readRegistry(`SOFTWARE\Classes\tg\shell\open\command`, "")
	if regPath != "" {
		path := extractExePath(regPath)
		if path != "" && fileExists(path) {
			return path
		}
	}

	// 3. %APPDATA%\Telegram Desktop
	appData := os.Getenv("APPDATA")
	if appData != "" {
		p := filepath.Join(appData, "Telegram Desktop", "Telegram.exe")
		if fileExists(p) {
			return p
		}
	}

	// 4. %LOCALAPPDATA%\Telegram Desktop
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		p := filepath.Join(localAppData, "Telegram Desktop", "Telegram.exe")
		if fileExists(p) {
			return p
		}
	}

	return ""
}

// extractExePath 从注册表命令字符串中提取 exe 路径
// 输入如: "C:\Path\Telegram.exe" "%1" 或 C:\Path\Telegram.exe %1
func extractExePath(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	if cmd[0] == '"' {
		end := strings.Index(cmd[1:], "\"")
		if end > 0 {
			return cmd[1 : 1+end]
		}
	} else {
		// 没有引号，取第一个空格前
		space := strings.Index(cmd, " ")
		if space > 0 {
			return cmd[:space]
		}
		return cmd
	}
	return ""
}

// ensureProxyConfig 预创建 TgWsProxy 的 config.json，写入固定 secret
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

// readRegistry 读取 Windows 注册表字符串值
// valueName 为空时读取默认值
func readRegistry(keyPath, valueName string) string {
	advapi32 := syscall.NewLazyDLL("advapi32.dll")
	regOpenKey := advapi32.NewProc("RegOpenKeyExW")
	regQueryValue := advapi32.NewProc("RegQueryValueExW")
	regCloseKey := advapi32.NewProc("RegCloseKey")

	for _, root := []uintptr{0x80000001, 0x80000002} { // HKCU, HKLM
		var hKey uintptr
		keyPathPtr, _ := syscall.UTF16PtrFromString(keyPath)
		ret, _, _ := regOpenKey.Call(root, uintptr(unsafe.Pointer(keyPathPtr)), 0, 0x20019, uintptr(unsafe.Pointer(&hKey)))
		if ret != 0 {
			continue
		}

		var bufLen uint32 = 2048
		buf := make([]uint16, bufLen)
		var valueNamePtr *uint16
		if valueName != "" {
			valueNamePtr, _ = syscall.UTF16PtrFromString(valueName)
		}
		var valType uint32
		ret2, _, _ := regQueryValue.Call(
			hKey,
			uintptr(unsafe.Pointer(valueNamePtr)),
			uintptr(unsafe.Pointer(&valType)),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(&bufLen)),
		)
		regCloseKey.Call(hKey)

		if ret2 == 0 && bufLen > 0 {
			return syscall.UTF16ToString(buf[:bufLen/2])
		}
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
