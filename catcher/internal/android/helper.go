package android

import (
	"crypto/md5"
	"crypto/x509"
	"encoding/asn1"
	"encoding/binary"
	"encoding/pem"
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
	su2Path := "/data/user/0/bin.mt.plus/files/term/bin/su2"
	proxyStr := fmt.Sprintf("%s:%d", host, port)

	// 关键：-c 后面只给一个字符串
	cmdStr := fmt.Sprintf("/system/bin/settings put global http_proxy %s", proxyStr)
	// 或者：fmt.Sprintf("/system/bin/settings put --user 0 global http_proxy %s", proxyStr)

	cmd := exec.Command("/system/bin/sh", su2Path, "-c", cmdStr)
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

// subjectHashOld 计算证书的 subject_hash_old (等价于 openssl x509 -subject_hash_old)
// Android 系统证书目录使用此哈希作为文件名
func subjectHashOld(cert *x509.Certificate) (string, error) {
	// subject_hash_old 使用 DER 编码的 Subject 的 MD5 哈希的前 4 字节 (小端序)
	subjectDER, err := asn1.Marshal(cert.Subject.ToRDNSequence())
	if err != nil {
		return "", fmt.Errorf("编码证书主题失败: %w", err)
	}

	hash := md5.Sum(subjectDER)
	// 取前 4 字节，按小端序读取为 uint32
	hashValue := binary.LittleEndian.Uint32(hash[:4])
	return fmt.Sprintf("%08x", hashValue), nil
}

// InstallCert 安装 CA 证书到系统证书目录
// 返回: installed=true 表示新安装了证书（需要重启生效）, false 表示已存在
func (h *AndroidHelper) InstallCert(certPath string) (bool, error) {
	if !h.IsRoot() {
		return false, fmt.Errorf("需要 Root 权限")
	}

	// 读取证书内容
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return false, fmt.Errorf("读取证书失败: %w", err)
	}

	// 解析 PEM 证书
	block, _ := pem.Decode(certData)
	if block == nil {
		return false, fmt.Errorf("无效的 PEM 证书文件")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, fmt.Errorf("解析证书失败: %w", err)
	}

	// 计算 subject_hash_old 作为文件名
	hashStr, err := subjectHashOld(cert)
	if err != nil {
		return false, fmt.Errorf("计算证书哈希失败: %w", err)
	}
	certName := hashStr + ".0"

	// 系统证书目录
	systemCertDir := "/system/etc/security/cacerts"
	destPath := filepath.Join(systemCertDir, certName)

	// 检查证书是否已安装 (用正确哈希名)
	if _, err := os.Stat(destPath); err == nil {
		fmt.Printf("[Android] CA 证书已安装: %s\n", destPath)
		return false, nil
	}

	// 重新挂载 /system 为可写
	if err := h.remountSystem(); err != nil {
		return false, fmt.Errorf("挂载系统分区失败: %w", err)
	}

	// 清理旧的错误命名的证书 (之前用的固定名)
	oldPath := filepath.Join(systemCertDir, "lunabot-catcher.0")
	if _, err := os.Stat(oldPath); err == nil {
		os.Remove(oldPath)
		fmt.Println("[Android] 已清理旧证书: lunabot-catcher.0")
	}

	// 写入证书 (使用正确的哈希文件名)
	if err := os.WriteFile(destPath, certData, 0644); err != nil {
		return false, fmt.Errorf("写入证书失败: %w", err)
	}

	fmt.Printf("[Android] CA 证书已安装: %s (hash: %s)\n", destPath, hashStr)
	return true, nil
}

// Reboot 重启设备
func (h *AndroidHelper) Reboot() error {
	fmt.Println("[Android] 证书已安装，3 秒后自动重启设备...")
	// 给用户一个反应时间
	cmd := exec.Command("sh", "-c", "sleep 3 && reboot")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("重启失败: %w", err)
	}
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
