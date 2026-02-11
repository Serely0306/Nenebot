package android

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AndroidHelper Android 系统操作辅助
type AndroidHelper struct {
	DataDir string
}

// NewAndroidHelper 创建 Android 辅助器
func NewAndroidHelper(dataDir string) *AndroidHelper {
	return &AndroidHelper{DataDir: dataDir}
}

// IsAndroid 检查是否在 Android 系统上运行
func (h *AndroidHelper) IsAndroid() bool {
	// 检查 /system/build.prop 是否存在
	_, err := os.Stat("/system/build.prop")
	return err == nil
}

// IsRoot 检查是否有 Root 权限
func (h *AndroidHelper) IsRoot() bool {
	return os.Getuid() == 0
}

// SetProxy 设置系统 HTTP 代理 (尝试多种方法)
func (h *AndroidHelper) SetProxy(host string, port int) error {
	proxyStr := fmt.Sprintf("%s:%d", host, port)
	var lastErr error
	success := false

	// 方法 1: settings put global http_proxy (Android 5+)
	cmd := exec.Command("settings", "put", "global", "http_proxy", proxyStr)
	if output, err := cmd.CombinedOutput(); err == nil {
		fmt.Printf("[Android] 方法1成功 - 已设置代理: %s\n", proxyStr)
		success = true
	} else {
		lastErr = fmt.Errorf("方法1失败: %s", string(output))
	}

	// 方法 2: 使用 su -c 执行 settings 命令 (某些虚拟机需要)
	if !success {
		cmd = exec.Command("su", "-c", fmt.Sprintf("settings put global http_proxy %s", proxyStr))
		if output, err := cmd.CombinedOutput(); err == nil {
			fmt.Printf("[Android] 方法2成功 - 已设置代理: %s\n", proxyStr)
			success = true
		} else {
			lastErr = fmt.Errorf("方法2失败: %s", string(output))
		}
	}

	// 方法 3: 使用 su2 (光速虚拟机特有)
	if !success {
		cmd = exec.Command("su2", "-c", fmt.Sprintf("settings put global http_proxy %s", proxyStr))
		if output, err := cmd.CombinedOutput(); err == nil {
			fmt.Printf("[Android] 方法3成功 (su2) - 已设置代理: %s\n", proxyStr)
			success = true
		} else {
			lastErr = fmt.Errorf("方法3失败: %s", string(output))
		}
	}

	// 方法 4: 直接设置系统属性 (旧版 Android)
	if !success {
		cmd = exec.Command("setprop", "net.gprs.http-proxy", proxyStr)
		if _, err := cmd.CombinedOutput(); err == nil {
			fmt.Printf("[Android] 方法4成功 (setprop) - 已设置代理: %s\n", proxyStr)
			success = true
		}
	}

	// 方法 5: 使用 content 命令 (Android 4.2+)
	if !success {
		// content insert --uri content://settings/global --bind name:s:http_proxy --bind value:s:host:port
		cmd = exec.Command("content", "insert", "--uri", "content://settings/global",
			"--bind", "name:s:http_proxy", "--bind", fmt.Sprintf("value:s:%s", proxyStr))
		if output, err := cmd.CombinedOutput(); err == nil {
			fmt.Printf("[Android] 方法5成功 (content) - 已设置代理: %s\n", proxyStr)
			success = true
		} else {
			lastErr = fmt.Errorf("方法5失败: %s", string(output))
		}
	}

	if !success {
		fmt.Printf("[Android] 警告: 自动设置代理失败，请手动设置\n")
		fmt.Printf("[Android] 请在 WiFi 设置中手动配置代理: %s\n", proxyStr)
		return lastErr
	}

	return nil
}

// ClearProxy 清除系统代理 (尝试多种方法)
func (h *AndroidHelper) ClearProxy() error {
	methods := []struct {
		name string
		cmd  *exec.Cmd
	}{
		{"settings", exec.Command("settings", "put", "global", "http_proxy", ":0")},
		{"su -c settings", exec.Command("su", "-c", "settings put global http_proxy :0")},
		{"su2 -c settings", exec.Command("su2", "-c", "settings put global http_proxy :0")},
		{"content delete", exec.Command("content", "delete", "--uri", "content://settings/global", "--where", "name='http_proxy'")},
	}

	for _, m := range methods {
		if output, err := m.cmd.CombinedOutput(); err == nil {
			fmt.Printf("[Android] 已清除代理 (%s)\n", m.name)
			return nil
		} else {
			_ = output // 忽略错误，尝试下一个方法
		}
	}

	fmt.Println("[Android] 警告: 清除代理可能失败，请手动检查")
	return nil
}

// InstallCert 安装 CA 证书到系统证书目录
func (h *AndroidHelper) InstallCert(certPath string) error {
	if !h.IsRoot() {
		return fmt.Errorf("需要 Root 权限")
	}

	// 获取证书所在目录
	certDir := filepath.Dir(certPath)

	// 查找 .0 格式的 Android 证书文件
	var androidCertPath string
	var androidCertName string

	files, err := os.ReadDir(certDir)
	if err != nil {
		return fmt.Errorf("读取证书目录失败: %w", err)
	}

	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".0") {
			androidCertPath = filepath.Join(certDir, f.Name())
			androidCertName = f.Name()
			break
		}
	}

	if androidCertPath == "" {
		// 如果没有 .0 文件，直接使用 PEM 证书并用简化名称
		androidCertPath = certPath
		androidCertName = "catcher-ca.0"
		fmt.Println("[Android] 未找到 .0 证书文件，将使用 PEM 证书创建")
	}

	// 读取证书内容
	certData, err := os.ReadFile(androidCertPath)
	if err != nil {
		return fmt.Errorf("读取证书失败: %w", err)
	}

	// 系统证书目录
	systemCertDir := "/system/etc/security/cacerts"
	destPath := filepath.Join(systemCertDir, androidCertName)

	// 检查证书是否已安装
	if _, err := os.Stat(destPath); err == nil {
		fmt.Printf("[Android] CA 证书已安装: %s\n", destPath)
		return nil
	}

	// 尝试多种方式重新挂载 /system 为可写
	if err := h.remountSystem(); err != nil {
		fmt.Printf("[Android] 警告: 标准挂载失败，尝试其他方法... (%v)\n", err)
		// 尝试其他挂载方式
		h.tryAlternativeRemount()
	}

	// 先复制到临时目录
	tmpPath := "/data/local/tmp/" + androidCertName
	if err := os.WriteFile(tmpPath, certData, 0644); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	// 使用 cp 命令复制到系统目录 (某些系统直接写入会失败)
	cmd := exec.Command("cp", tmpPath, destPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		// 尝试使用 su -c
		cmd = exec.Command("su", "-c", fmt.Sprintf("cp %s %s", tmpPath, destPath))
		if output2, err2 := cmd.CombinedOutput(); err2 != nil {
			// 尝试使用 su2 -c (光速虚拟机)
			cmd = exec.Command("su2", "-c", fmt.Sprintf("cp %s %s && chmod 644 %s", tmpPath, destPath, destPath))
			if output3, err3 := cmd.CombinedOutput(); err3 != nil {
				return fmt.Errorf("安装证书失败: %s / %s / %s", string(output), string(output2), string(output3))
			}
		}
	}

	// 设置权限
	exec.Command("chmod", "644", destPath).Run()
	exec.Command("su", "-c", fmt.Sprintf("chmod 644 %s", destPath)).Run()

	// 清理临时文件
	os.Remove(tmpPath)

	fmt.Printf("[Android] CA 证书已安装到: %s\n", destPath)
	fmt.Println("[Android] 注意: 可能需要重启设备才能生效")
	return nil
}

// tryAlternativeRemount 尝试其他挂载方式
func (h *AndroidHelper) tryAlternativeRemount() {
	// 方法 1: 使用 su -c
	exec.Command("su", "-c", "mount -o rw,remount /system").Run()
	// 方法 2: 使用 su2 (光速虚拟机)
	exec.Command("su2", "-c", "mount -o rw,remount /system").Run()
	// 方法 3: 挂载 /
	exec.Command("mount", "-o", "rw,remount", "/").Run()
}

// remountSystem 重新挂载系统分区为可写
func (h *AndroidHelper) remountSystem() error {
	cmd := exec.Command("mount", "-o", "rw,remount", "/system")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}
	return nil
}

// GetLocalIP 获取本机 IP 地址
func (h *AndroidHelper) GetLocalIP() string {
	// 尝试通过连接外部服务器获取本机 IP
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// SetupDNS 设置 DNS (某些虚拟机需要)
func (h *AndroidHelper) SetupDNS() error {
	if !h.IsRoot() {
		return nil
	}

	resolvConf := "/etc/resolv.conf"

	// 检查是否需要创建
	content, err := os.ReadFile(resolvConf)
	if err != nil || !strings.Contains(string(content), "223.5.5.5") {
		// 重新挂载系统分区
		h.remountSystem()

		// 写入阿里 DNS
		dnsConfig := "nameserver 223.5.5.5\nnameserver 223.6.6.6\n"
		if err := os.WriteFile(resolvConf, []byte(dnsConfig), 0644); err != nil {
			return err
		}
		fmt.Println("[Android] DNS 已配置")
	}

	return nil
}
