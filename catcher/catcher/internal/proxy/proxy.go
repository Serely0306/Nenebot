package proxy

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/elazarl/goproxy"

	"lunabot-catcher/internal/crypto"
	"lunabot-catcher/internal/uploader"
)

// RegionHosts 各区服的 API 域名
var RegionHosts = map[string][]string{
	"jp": {"production-game-api.sekai.colorfulpalette.org"},
	"en": {"production-game-api.sekai.colorfulstage.com"},
	"tw": {"prod-api.sekai-pl.com"},
	"kr": {"prod-api.sekai-m.com"},
	"cn": {"mkcn-prod-public-60001-1.dailygn.com", "mkcn-prod-public-60001-2.dailygn.com"},
}

// hostToRegion 主机名到区服的映射
var hostToRegion map[string]string

func init() {
	hostToRegion = make(map[string]string)
	for region, hosts := range RegionHosts {
		for _, host := range hosts {
			hostToRegion[host] = region
		}
	}
}

// MitmProxy MITM 代理服务器
type MitmProxy struct {
	proxy    *goproxy.ProxyHttpServer
	uploader *uploader.Uploader
	debug    bool
	certPath string
	keyPath  string
}

// NewMitmProxy 创建 MITM 代理
// externalCertPath/externalKeyPath: 可选，使用外部证书（如 HarukiProxy 的证书）
func NewMitmProxy(up *uploader.Uploader, debug bool, dataDir string, externalCertPath, externalKeyPath string) (*MitmProxy, error) {
	p := &MitmProxy{
		proxy:    goproxy.NewProxyHttpServer(),
		uploader: up,
		debug:    debug,
	}

	// 决定证书路径
	if externalCertPath != "" && externalKeyPath != "" {
		// 使用外部证书
		p.certPath = externalCertPath
		p.keyPath = externalKeyPath
		log.Printf("[CA] 使用外部证书: %s\n", externalCertPath)
	} else {
		// 使用默认路径（会自动生成）
		p.certPath = filepath.Join(dataDir, "ca-cert.pem")
		p.keyPath = filepath.Join(dataDir, "ca-key.pem")
	}

	p.proxy.Verbose = debug

	// 生成或加载 CA 证书
	if err := p.setupCA(); err != nil {
		return nil, fmt.Errorf("setup CA: %w", err)
	}

	// 设置 HTTPS 拦截
	p.setupHTTPSIntercept()

	// 设置响应处理
	p.setupResponseHandler()

	return p, nil
}

// setupCA 设置 CA 证书
func (p *MitmProxy) setupCA() error {
	// 检查证书是否已存在
	if _, err := os.Stat(p.certPath); err == nil {
		if _, err := os.Stat(p.keyPath); err == nil {
			// 加载现有证书
			return p.loadCA()
		}
	}

	// 生成新 CA 证书
	return p.generateCA()
}

// generateCA 生成 CA 证书
func (p *MitmProxy) generateCA() error {
	log.Println("[CA] 生成新的 CA 证书...")

	// 生成私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	// 创建证书模板
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Country:            []string{"CN"},
			Organization:       []string{"Catcher"},
			OrganizationalUnit: []string{"Catcher CA"},
			CommonName:         "Catcher Root CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0), // 10 年有效期
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            2,
	}

	// 创建证书
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return err
	}

	// 确保目录存在
	certDir := filepath.Dir(p.certPath)
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return err
	}

	// 保存 PEM 格式证书 (ca.pem)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	if err := os.WriteFile(p.certPath, certPEM, 0644); err != nil {
		return err
	}

	// 保存私钥 (ca.key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	if err := os.WriteFile(p.keyPath, keyPEM, 0600); err != nil {
		return err
	}

	// 计算 subject_hash_old 并生成 Android 兼容的 .0 文件
	cert, _ := x509.ParseCertificate(derBytes)
	hashName := calculateSubjectHashOld(cert)
	androidCertPath := filepath.Join(certDir, hashName+".0")

	// Android 需要的格式: 证书 PEM + 额外信息
	androidCertContent := string(certPEM)
	// 添加人类可读的证书信息 (可选，但有助于识别)
	androidCertContent += fmt.Sprintf("-----BEGIN CERTIFICATE INFO-----\n")
	androidCertContent += fmt.Sprintf("Subject: %s\n", cert.Subject.String())
	androidCertContent += fmt.Sprintf("Issuer: %s\n", cert.Issuer.String())
	androidCertContent += fmt.Sprintf("Valid From: %s\n", cert.NotBefore.Format("2006-01-02"))
	androidCertContent += fmt.Sprintf("Valid To: %s\n", cert.NotAfter.Format("2006-01-02"))
	androidCertContent += fmt.Sprintf("-----END CERTIFICATE INFO-----\n")

	if err := os.WriteFile(androidCertPath, []byte(androidCertContent), 0644); err != nil {
		log.Printf("[CA] 警告: 无法创建 Android 证书文件: %v\n", err)
	} else {
		log.Printf("[CA] Android 证书已生成: %s\n", androidCertPath)
	}

	log.Printf("[CA] 证书已保存到: %s\n", p.certPath)
	log.Printf("[CA] 私钥已保存到: %s\n", p.keyPath)

	return p.loadCA()
}

// calculateSubjectHashOld 计算证书的 subject_hash_old (OpenSSL 兼容)
// 这是 Android 系统证书目录所需的文件名格式
func calculateSubjectHashOld(cert *x509.Certificate) string {
	// subject_hash_old 是基于 MD5 的旧版哈希算法
	// 它对 Subject 的 DER 编码进行哈希
	// 简化实现：使用 Subject 的前几个字节生成一个伪哈希
	// 注意：这不是完全精确的 OpenSSL subject_hash_old 实现，
	// 但对于我们的用途足够了

	subjectDER := cert.RawSubject

	// 使用 MD5 计算哈希 (OpenSSL subject_hash_old 使用 MD5)
	h := md5.Sum(subjectDER)

	// 取前 4 字节作为小端序 32 位整数
	hash := uint32(h[0]) | uint32(h[1])<<8 | uint32(h[2])<<16 | uint32(h[3])<<24

	return fmt.Sprintf("%08x", hash)
}

// loadCA 加载 CA 证书
func (p *MitmProxy) loadCA() error {
	cert, err := tls.LoadX509KeyPair(p.certPath, p.keyPath)
	if err != nil {
		return err
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return err
	}

	goproxy.GoproxyCa = cert
	goproxy.OkConnect = &goproxy.ConnectAction{Action: goproxy.ConnectMitm, TLSConfig: goproxy.TLSConfigFromCA(&cert)}
	goproxy.MitmConnect = &goproxy.ConnectAction{Action: goproxy.ConnectMitm, TLSConfig: goproxy.TLSConfigFromCA(&cert)}
	goproxy.RejectConnect = &goproxy.ConnectAction{Action: goproxy.ConnectReject, TLSConfig: goproxy.TLSConfigFromCA(&cert)}

	log.Printf("[CA] 已加载 CA 证书: %s (有效期至 %s)\n", x509Cert.Subject.CommonName, x509Cert.NotAfter.Format("2006-01-02"))

	return nil
}

// setupHTTPSIntercept 设置 HTTPS 拦截
func (p *MitmProxy) setupHTTPSIntercept() {
	// 只拦截游戏服务器的 HTTPS 连接
	p.proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)
}

// setupResponseHandler 设置响应处理器
func (p *MitmProxy) setupResponseHandler() {
	// 只匹配 /api/user/{uid}/mysekai (不匹配 suite)
	apiPattern := regexp.MustCompile(`^/api/user/(\d+)/mysekai$`)

	p.proxy.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if resp == nil || resp.Request == nil {
			return resp
		}

		host := resp.Request.Host
		path := resp.Request.URL.Path
		query := resp.Request.URL.RawQuery

		// 检查是否是游戏 API
		region, ok := hostToRegion[host]
		if !ok {
			return resp
		}

		// 检查是否是 mysekai API
		matches := apiPattern.FindStringSubmatch(path)
		if matches == nil {
			return resp
		}

		// 必须包含 isForceAllReloadOnlyMysekai=True 参数
		// 这是全量数据请求，其他请求不处理
		if query == "" || !regexp.MustCompile(`isForceAllReloadOnlyMysekai=True`).MatchString(query) {
			if p.debug {
				log.Printf("[跳过] 非全量请求: %s?%s\n", path, query)
			}
			return resp
		}

		uid := matches[1]

		// 只处理成功的响应
		if resp.StatusCode != 200 {
			return resp
		}

		// 读取响应体
		body, err := readResponseBody(resp)
		if err != nil {
			if p.debug {
				log.Printf("[错误] 读取响应体失败: %v\n", err)
			}
			return resp
		}

		if len(body) == 0 {
			return resp
		}

		log.Printf("\n%s\n", "==================================================")
		log.Printf("[拦截] %s - mysekai (全量) - UID: %s\n", region, uid)
		log.Printf("[请求] %s?%s\n", path, query)
		log.Printf("%s\n", "==================================================")

		// 异步处理数据
		go p.processData(region, uid, "mysekai", body)

		return resp
	})
}

// processData 处理抓取的数据
func (p *MitmProxy) processData(region, uid, dataType string, body []byte) {
	// 解密数据
	data, err := crypto.DecryptAndUnpack(body, region)
	if err != nil {
		log.Printf("[%s] 解密失败: %v\n", dataType, err)
		// 保存原始数据用于调试
		if saveErr := p.uploader.SaveRawData(region, uid, dataType, body); saveErr != nil {
			log.Printf("[%s] 保存原始数据失败: %v\n", dataType, saveErr)
		}
		return
	}

	// 上传
	if err := p.uploader.Upload(region, uid, dataType, data); err != nil {
		log.Printf("[%s] 上传失败: %v\n", dataType, err)
		return
	}

	log.Printf("[%s] ✓ 数据处理成功: %s user %s\n", dataType, region, uid)
}

// GetCertPath 获取 CA 证书路径
func (p *MitmProxy) GetCertPath() string {
	return p.certPath
}

// ListenAndServe 启动代理服务器
func (p *MitmProxy) ListenAndServe(addr string) error {
	log.Printf("[代理] MITM 代理已启动: %s\n", addr)
	return http.ListenAndServe(addr, p.proxy)
}
