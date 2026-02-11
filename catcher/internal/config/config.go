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

	// 证书配置 (可选，留空则自动生成)
	// 如果想复用 HarukiProxy 的证书，可以设置这两个路径
	ExternalCertPath string `yaml:"external_cert_path"`
	ExternalKeyPath  string `yaml:"external_key_path"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Listen:           "0.0.0.0:8888",
		UploadServer:     "http://127.0.0.1:5000",
		AutoUpload:       true,
		SaveLocally:      false,
		SaveDir:          "./data",
		AndroidProxyIP:   "",
		Debug:            false,
		ExternalCertPath: "",
		ExternalKeyPath:  "",
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
