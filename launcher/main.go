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

// 默认配置
const (
	DefaultProxyPort   = 1443
	DefaultProxyHost   = "127.0.0.1"
	DefaultProxySecret = "ee8a1b8e0c2d4f6a9b3e7c5d1f2a4b6c"
)

// LauncherConfig 启动器自己的配置文件
type LauncherConfig struct {
	// Telegram.exe 的路径，相对于启动器目录或绝对路径
	// 留空 = 自动查找（同级 Telegram\Telegram.exe → 注册表 → PATH）
	TelegramPath string `json:"telegram_path"`

	// TgWsProxy.exe 的路径，相对于启动器目录或绝对路径
	// 留空 = wsproxy\TgWsProxy.exe
	ProxyPath string `json:"proxy_path"`

	// 代理监听端口
	ProxyPort int `json:"proxy_port"`

	// 代理监听地址
	ProxyHost string `json:"proxy_host"`

	// MTProto 密钥，留空 = 用固定默认值
	ProxySecret string `json:"proxy_secret"`
}

func main() {
	exePath, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	rootDir := filepath.Dir(exePath)

	// 0. 加载配置
	cfg := loadConfig(rootDir)

	// 1. 查找 TgWsProxy.exe
	proxyExe := resolvePath(cfg.ProxyPath, rootDir, filepath.Join("wsproxy", "TgWsProxy.exe"))
	if _, err := os.Stat(proxyExe); os.IsNotExist(err) {
		msgBox("错误", fmt.Sprintf("未找到代理程序:\n%s\n\n请在 config.json 中设置 proxy_path，或将 TgWsProxy.exe 放入 wsproxy 目录。", proxyExe))
		os.Exit(1)
	}

	// 2. 查找 Telegram.exe
	tgExe := findTelegram(cfg.TelegramPath, rootDir)
	if tgExe == "" {
		msgBox("错误", "未找到 Telegram.exe\n\n请将 Telegram 放入同级 Telegram/ 目录，\n或在 config.json 中设置 telegram_path。")
		os.Exit(1)
	}

	// 3. 预创建代理配置文件
	ensureProxyConfig(cfg)

	// 4. 启动代理
	proxyAlreadyRunning := processExists("TgWsProxy.exe")
	if !proxyAlreadyRunning {
		proxyCmd := exec.Command(proxyExe)
		proxyCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
		proxyCmd.Start()
	}

	// 5. 等待代理端口就绪
	port := cfg.ProxyPort
	if port == 0 {
		port = DefaultProxyPort
	}
	host := cfg.ProxyHost
	if host == "" {
		host = DefaultProxyHost
	}
	portReady := waitForPort(host, port, 15*time.Second)
	if !portReady {
		msgBox("提示", "代理启动较慢，正在尝试启动 Telegram…")
	}

	// 6. 读取 secret
	secret := cfg.ProxySecret
	if secret == "" {
		secret = DefaultProxySecret
	}

	// 7. 首次运行配置 Telegram
	markerFile := filepath.Join(rootDir, ".proxy_configured")
	savedSecret, _ := os.ReadFile(markerFile)
	savedSecretStr := strings.TrimSpace(string(savedSecret))

	if savedSecretStr != secret {
		// 首次运行或 secret 变了
		if processExists("Telegram.exe") {
			killProcess("Telegram.exe")
			time.Sleep(2 * time.Second)
		}
		// 打开 tg://proxy 深度链接
		openURL(fmt.Sprintf("tg://proxy?server=%s&port=%d&secret=%s", host, port, secret))
		time.Sleep(5 * time.Second)
		os.WriteFile(markerFile, []byte(secret), 0644)
	} else {
		// 后续运行：直接启动 Telegram
		if !processExists("Telegram.exe") {
			cmd := exec.Command(tgExe)
			cmd.Dir = filepath.Dir(tgExe)
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
			cmd.Start()
		}
	}

	// 8. 等待 Telegram 退出
	for {
		time.Sleep(2 * time.Second)
		if !processExists("Telegram.exe") {
			break
		}
	}

	// 9. 关闭代理
	if !proxyAlreadyRunning {
		killProcess("TgWsProxy.exe")
	}
	os.Exit(0)
}

// loadConfig 从启动器目录读取 config.json，不存在则创建默认配置
func loadConfig(rootDir string) LauncherConfig {
	configPath := filepath.Join(rootDir, "config.json")

	cfg := LauncherConfig{}

	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, &cfg)
		return cfg
	}

	// 创建默认配置文件
	cfg = LauncherConfig{
		TelegramPath: "",
		ProxyPath:    "",
		ProxyPort:    DefaultProxyPort,
		ProxyHost:    DefaultProxyHost,
		ProxySecret:  DefaultProxySecret,
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(configPath, data, 0644)

	return cfg
}

// resolvePath 解析路径：如果是绝对路径直接用，否则拼接 rootDir
func resolvePath(p, rootDir, defaultRel string) string {
	if p == "" {
		return filepath.Join(rootDir, defaultRel)
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(rootDir, p)
}

// findTelegram 按优先级查找 Telegram.exe:
// 1. 配置文件指定的路径
// 2. 同级 Telegram\Telegram.exe
// 3. 注册表 HKLM\...\Telegram Desktop\InstallPath
// 4. %APPDATA%\Telegram Desktop\Telegram.exe (便携版常见位置)
// 5. PATH 环境变量
func findTelegram(configuredPath, rootDir string) string {
	// 1. 配置文件
	if configuredPath != "" {
		p := configuredPath
		if !filepath.IsAbs(p) {
			p = filepath.Join(rootDir, p)
		}
		if fileExists(p) {
			return p
		}
	}

	// 2. 同级目录
	localPath := filepath.Join(rootDir, "Telegram", "Telegram.exe")
	if fileExists(localPath) {
		return localPath
	}

	// 3. 注册表
	regPath := readRegistry(`SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\{53F49750-6203-4FB3-9D28-A6A0C8ACF38F}`, "InstallLocation")
	if regPath != "" {
		p := filepath.Join(regPath, "Telegram.exe")
		if fileExists(p) {
			return p
		}
	}
	// 也试 HKCU
	regPath = readRegistry(`SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\{53F49750-6203-4FB3-9D28-A6A0C8ACF38F}`, "InstallLocation")
	if regPath != "" {
		p := filepath.Join(regPath, "Telegram.exe")
		if fileExists(p) {
			return p
		}
	}

	// 4. %APPDATA%\Telegram Desktop
	appData := os.Getenv("APPDATA")
	if appData != "" {
		p := filepath.Join(appData, "Telegram Desktop", "Telegram.exe")
		if fileExists(p) {
			return p
		}
	}

	// 5. %LOCALAPPDATA%\Telegram Desktop
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		p := filepath.Join(localAppData, "Telegram Desktop", "Telegram.exe")
		if fileExists(p) {
			return p
		}
	}

	// 6. where 命令
	out, err := exec.Command("where", "Telegram.exe").Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 0 {
			p := strings.TrimSpace(lines[0])
			if fileExists(p) {
				return p
			}
		}
	}

	return ""
}

// readRegistry 读取 Windows 注册表字符串值
func readRegistry(keyPath, valueName string) string {
	advapi32 := syscall.NewLazyDLL("advapi32.dll")
	regOpenKey := advapi32.NewProc("RegOpenKeyExW")
	regQueryValue := advapi32.NewProc("RegQueryValueExW")
	regCloseKey := advapi32.NewProc("RegCloseKey")

	// 尝试 HKLM (0x80000002) 和 HKCU (0x80000001)
	for _, root := []uintptr{0x80000002, 0x80000001} {
		var hKey uintptr
		keyPathPtr, _ := syscall.UTF16PtrFromString(keyPath)
		ret, _, _ := regOpenKey.Call(root, uintptr(unsafe.Pointer(keyPathPtr)), 0, 0x20019, uintptr(unsafe.Pointer(&hKey)))
		if ret != 0 {
			continue
		}

		var bufLen uint32 = 1024
		buf := make([]uint16, bufLen)
		valueNamePtr, _ := syscall.UTF16PtrFromString(valueName)
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

// ensureProxyConfig 预创建 TgWsProxy 的 config.json
func ensureProxyConfig(cfg LauncherConfig) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return
	}

	port := cfg.ProxyPort
	if port == 0 {
		port = DefaultProxyPort
	}
	host := cfg.ProxyHost
	if host == "" {
		host = DefaultProxyHost
	}
	secret := cfg.ProxySecret
	if secret == "" {
		secret = DefaultProxySecret
	}

	configDir := filepath.Join(appData, "TgWsProxy")
	os.MkdirAll(configDir, 0755)

	configPath := filepath.Join(configDir, "config.json")

	// 如果配置已存在且包含 secret，不覆盖
	if data, err := os.ReadFile(configPath); err == nil {
		var config map[string]interface{}
		if json.Unmarshal(data, &config) == nil {
			if s, ok := config["secret"].(string); ok && s != "" {
				// secret 已存在，不覆盖
				return
			}
		}
	}

	config := map[string]interface{}{
		"port":             port,
		"host":             host,
		"secret":           secret,
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
