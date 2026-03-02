package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Config 应用配置
type Config struct {
	Listen         string `yaml:"listen"`
	UploadServer   string `yaml:"upload_server"`
	AutoUpload     bool   `yaml:"auto_upload"`
	SaveLocally    bool   `yaml:"save_locally"`
	SaveDir        string `yaml:"save_dir"`
	AndroidProxyIP string `yaml:"android_proxy_ip"`
	Debug          bool   `yaml:"debug"`

	// 抓取内容控制
	CaptureMysekai bool `yaml:"capture_mysekai"`
	CaptureSuite   bool `yaml:"capture_suite"`

	// 强制全量刷新 MySekai：将请求中的 isForceAllReloadOnlyMysekai=False 篡改为 True
	// 开启后每次进入 MySekai 页面都会获取完整数据（而不是仅第一次进入）
	// 开启此项会自动启用 capture_mysekai
	ForceMysekaiReload bool `yaml:"force_mysekai_reload"`

	// MITM 控制：仅对游戏 API 域名执行 MITM（推荐开启）
	MitmTargetOnly bool `yaml:"mitm_target_only"`

	// 证书配置 (可选，留空则自动生成)
	// 如果想复用 HarukiProxy 的证书，可以设置这两个路径
	ExternalCertPath string `yaml:"external_cert_path"`
	ExternalKeyPath  string `yaml:"external_key_path"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Listen:             "0.0.0.0:8888",
		UploadServer:       "",
		AutoUpload:         true,
		SaveLocally:        false,
		SaveDir:            "./data",
		AndroidProxyIP:     "",
		Debug:              false,
		CaptureMysekai:     false,
		CaptureSuite:       true,
		ForceMysekaiReload: false,
		MitmTargetOnly:     true,
		ExternalCertPath:   "",
		ExternalKeyPath:    "",
	}
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 配置文件不存在，使用默认配置
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
