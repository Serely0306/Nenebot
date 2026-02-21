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

// SetProxy 设置系统 HTTP 代理
//
//	func (h *AndroidHelper) SetProxy(host string, port int) error {
//		proxyStr := fmt.Sprintf("%s:%d", host, port)
//		cmd := exec.Command("settings", "put", "global", "http_proxy", proxyStr)
//		output, err := cmd.CombinedOutput()
//		if err != nil {
//			return fmt.Errorf("设置代理失败: %s", string(output))
//		}
//		fmt.Printf("[Android] 已设置代理: %s\n", proxyStr)
//		return nil
//	}
func (h *AndroidHelper) SetProxy(host string, port int) error {
	proxyStr := fmt.Sprintf("%s:%d", host, port)
	cmdStr := fmt.Sprintf("/system/bin/settings put --user 0 global http_proxy %s", proxyStr)

	cmd := exec.Command("su2", "-c", cmdStr) // 关键：su2 -c
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("设置代理失败: err=%v, out=%s", err, string(out))
	}
	return nil
}

// ClearProxy 清除系统代理
func (h *AndroidHelper) ClearProxy() error {
	cmd := exec.Command("settings", "put", "global", "http_proxy", ":0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("清除代理失败: %s", string(output))
	}
	fmt.Println("[Android] 已清除代理")
	return nil
}

// InstallCert 安装 CA 证书到系统证书目录
func (h *AndroidHelper) InstallCert(certPath string) error {
	if !h.IsRoot() {
		return fmt.Errorf("需要 Root 权限")
	}

	// 读取证书内容
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("读取证书失败: %w", err)
	}

	// 计算证书哈希 (用于文件名)
	// Android 使用 subject_hash_old 格式
	// 这里简化处理，使用固定名称
	certName := "lunabot-catcher.0"

	// 系统证书目录
	systemCertDir := "/system/etc/security/cacerts"
	destPath := filepath.Join(systemCertDir, certName)

	// 检查证书是否已安装
	if _, err := os.Stat(destPath); err == nil {
		fmt.Println("[Android] CA 证书已安装")
		return nil
	}

	// 重新挂载 /system 为可写
	if err := h.remountSystem(); err != nil {
		return fmt.Errorf("挂载系统分区失败: %w", err)
	}

	// 写入证书
	if err := os.WriteFile(destPath, certData, 0644); err != nil {
		return fmt.Errorf("写入证书失败: %w", err)
	}

	fmt.Printf("[Android] CA 证书已安装到: %s\n", destPath)
	return nil
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
