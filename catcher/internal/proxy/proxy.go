package proxy

import (
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
	"strings"
	"time"

	"github.com/elazarl/goproxy"

	"lunabot-catcher/internal/config"
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
	config   *config.Config
	debug    bool
	certPath string
	keyPath  string
}

// NewMitmProxy 创建 MITM 代理
// cfg: 应用配置，控制抓取内容和 MITM 行为
func NewMitmProxy(up *uploader.Uploader, cfg *config.Config, dataDir string) (*MitmProxy, error) {
	p := &MitmProxy{
		proxy:    goproxy.NewProxyHttpServer(),
		uploader: up,
		config:   cfg,
		debug:    cfg.Debug,
	}

	// 决定证书路径
	if cfg.ExternalCertPath != "" && cfg.ExternalKeyPath != "" {
		// 使用外部证书
		p.certPath = cfg.ExternalCertPath
		p.keyPath = cfg.ExternalKeyPath
		log.Printf("[CA] 使用外部证书: %s\n", cfg.ExternalCertPath)
	} else {
		// 使用默认路径（会自动生成）
		p.certPath = filepath.Join(dataDir, "ca-cert.pem")
		p.keyPath = filepath.Join(dataDir, "ca-key.pem")
	}

	p.proxy.Verbose = cfg.Debug

	// 生成或加载 CA 证书
	if err := p.setupCA(); err != nil {
		return nil, fmt.Errorf("setup CA: %w", err)
	}

	// 设置 HTTPS 拦截
	p.setupHTTPSIntercept()

	// 设置响应处理
	p.setupResponseHandler()

	// 设置请求修改器（强制全量刷新）
	p.setupRequestModifier()

	// ForceMysekaiReload 隐式启用 CaptureMysekai
	captureMysekai := cfg.CaptureMysekai || cfg.ForceMysekaiReload

	// 打印抓取配置（仅打印已启用的功能）
	if captureMysekai {
		log.Printf("[配置] MySekai 抓取: 已启用")
	}
	if cfg.ForceMysekaiReload {
		log.Printf("[配置] MySekai 强制全量刷新: 已启用")
	}
	if cfg.CaptureSuite {
		log.Printf("[配置] Suite 抓取: 已启用")
	}
	if cfg.MitmTargetOnly {
		log.Printf("[配置] MITM 仅目标域名: 已启用")
	}

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
			Organization: []string{"LunaBot Catcher"},
			CommonName:   "LunaBot Catcher CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0), // 10 年有效期
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// 创建证书
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return err
	}

	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(p.certPath), 0755); err != nil {
		return err
	}

	// 保存证书
	certFile, err := os.Create(p.certPath)
	if err != nil {
		return err
	}
	defer certFile.Close()
	pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	// 保存私钥（权限设为 0600，仅当前用户可读写）
	keyFile, err := os.OpenFile(p.keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer keyFile.Close()
	pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	log.Printf("[CA] 证书已保存到: %s\n", p.certPath)

	return p.loadCA()
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

// buildTargetHostMatcher 构建游戏 API 域名匹配器
func buildTargetHostMatcher() func(host string, ctx *goproxy.ProxyCtx) bool {
	return func(host string, ctx *goproxy.ProxyCtx) bool {
		// 去掉端口号
		h := host
		if idx := strings.Index(h, ":"); idx != -1 {
			h = h[:idx]
		}
		_, ok := hostToRegion[h]
		return ok
	}
}

// setupHTTPSIntercept 设置 HTTPS 拦截
func (p *MitmProxy) setupHTTPSIntercept() {
	if p.config.MitmTargetOnly {
		// 仅对游戏 API 域名执行 MITM，其他 HTTPS 流量直接透传
		p.proxy.OnRequest(goproxy.ReqConditionFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) bool {
			return buildTargetHostMatcher()(req.Host, ctx)
		})).HandleConnect(goproxy.AlwaysMitm)
		log.Println("[代理] MITM 模式: 仅目标域名")
	} else {
		// 拦截所有 HTTPS 连接
		p.proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)
		log.Println("[代理] MITM 模式: 全部流量")
	}
}

// setupRequestModifier 设置请求修改器（强制全量刷新）
func (p *MitmProxy) setupRequestModifier() {
	if !p.config.ForceMysekaiReload {
		return
	}

	mysekaiPattern := regexp.MustCompile(`^/api/user/\d+/mysekai$`)

	p.proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		if req == nil {
			return req, nil
		}

		// 检查是否是游戏 API 域名
		host := req.Host
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}
		if _, ok := hostToRegion[host]; !ok {
			return req, nil
		}

		// 检查是否是 mysekai 请求
		if !mysekaiPattern.MatchString(req.URL.Path) {
			return req, nil
		}

		// 检查是否包含 isForceAllReloadOnlyMysekai=False
		query := req.URL.Query()
		if query.Get("isForceAllReloadOnlyMysekai") == "False" {
			query.Set("isForceAllReloadOnlyMysekai", "True")
			req.URL.RawQuery = query.Encode()
			log.Printf("[强制刷新] 已将 %s 的 isForceAllReloadOnlyMysekai 修改为 True\n", req.URL.Path)
		}

		return req, nil
	})
}

// setupResponseHandler 设置响应处理器
func (p *MitmProxy) setupResponseHandler() {
	// MySekai: /api/user/{uid}/mysekai
	mysekaiPattern := regexp.MustCompile(`^/api/user/(\d+)/mysekai$`)
	// Suite: /api/suite/user/{uid}
	suitePattern := regexp.MustCompile(`^/api/suite/user/(\d+)$`)
	// MySekai 全量请求参数校验
	mysekaiForcePattern := regexp.MustCompile(`isForceAllReloadOnlyMysekai=True`)

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

		// 只处理成功的响应
		if resp.StatusCode != 200 {
			return resp
		}

		// 尝试匹配 MySekai API
		// ForceMysekaiReload 开启时隐式启用抓取
		if p.config.CaptureMysekai || p.config.ForceMysekaiReload {
			if matches := mysekaiPattern.FindStringSubmatch(path); matches != nil {
				// 仅抓取 isForceAllReloadOnlyMysekai=True（全量数据）
				// False 请求返回增量数据，字段不完整，保存会覆盖已有完整数据
				if query == "" || !mysekaiForcePattern.MatchString(query) {
					if p.debug {
						log.Printf("[跳过] 非 mysekai 全量请求: %s?%s\n", path, query)
					}
					return resp
				}

				uid := matches[1]

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
				log.Printf("[拦截] %s - mysekai - UID: %s\n", region, uid)
				log.Printf("[请求] %s?%s\n", path, query)
				log.Printf("%s\n", "==================================================")

				go p.processData(region, uid, "mysekai", body)
				return resp
			}
		}

		// 尝试匹配 Suite API
		if p.config.CaptureSuite {
			if matches := suitePattern.FindStringSubmatch(path); matches != nil {
				uid := matches[1]

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
				log.Printf("[拦截] %s - suite - UID: %s\n", region, uid)
				log.Printf("[请求] %s\n", path)
				log.Printf("%s\n", "==================================================")

				go p.processData(region, uid, "suite", body)
				return resp
			}
		}

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
