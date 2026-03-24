package onebotfilter

import (
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

type Config struct {
	Server  ServerConfig    `mapstructure:"server" yaml:"server"`
	BotApps []BotAppsConfig `mapstructure:"bot-apps" yaml:"bot-apps"`
}

type ServerConfig struct {
	Host        string            `mapstructure:"host" yaml:"host"`
	Port        uint              `mapstructure:"port" yaml:"port"`
	Suffix      string            `mapstructure:"suffix" yaml:"suffix"`
	BotId       string            `mapstructure:"bot-id" yaml:"bot-id"`
	UserAgent   string            `mapstructure:"user-agent" yaml:"user-agent"`
	Default     DefaultConfig     `mapstructure:"default" yaml:"default"`
	BufferSize  int               `mapstructure:"buffer-size" yaml:"buffer-size"`
	SleepTime   float32           `mapstructure:"sleep-time" yaml:"sleep-time"`
	Debug       bool              `mapstructure:"debug" yaml:"debug"`
	AccessToken string            `mapstructure:"access-token" yaml:"access-token"`
	Help        HelpConfig        `mapstructure:"help" yaml:"help"`
	FileServer  FileServerConfig  `mapstructure:"file-server" yaml:"file-server"`
	CommandAuth CommandAuthConfig `mapstructure:"command-auth" yaml:"command-auth"`
	Blocked     BlockedConfig     `mapstructure:"blocked" yaml:"blocked"`
}

type DefaultConfig struct {
	Private DefaultMessageTypeConfig `mapstructure:"private" yaml:"private"`
	Group   DefaultMessageTypeConfig `mapstructure:"group" yaml:"group"`
}

type DefaultMessageTypeConfig struct {
	Mode string  `mapstructure:"mode" yaml:"mode"`
	Ids  []int64 `mapstructure:"ids" yaml:"ids"`
}

type HelpConfig struct {
	Image           string `mapstructure:"image" yaml:"image"`
	Font            string `mapstructure:"font" yaml:"font"`
	Generate        bool   `mapstructure:"generate" yaml:"generate"`
	ForwardNickname string `mapstructure:"forward-nickname" yaml:"forward-nickname"`
}

type FileServerConfig struct {
	Enabled       bool   `mapstructure:"enabled" yaml:"enabled"`
	Base64Enabled bool   `mapstructure:"base64-enabled" yaml:"base64-enabled"`
	Base64MaxSize int64  `mapstructure:"base64-max-size" yaml:"base64-max-size"`
	Root          string `mapstructure:"root" yaml:"root"`
	PublicURL     string `mapstructure:"public-url" yaml:"public-url"`
}

type CommandAuthConfig struct {
	Enabled    bool    `mapstructure:"enabled" yaml:"enabled"`
	AllowOwner bool    `mapstructure:"allow-owner" yaml:"allow-owner"`
	AllowAdmin bool    `mapstructure:"allow-admin" yaml:"allow-admin"`
	SuperUsers []int64 `mapstructure:"super-users" yaml:"super-users"`
}

type BlockedConfig struct {
	UserIDs  []int64 `mapstructure:"user-ids" yaml:"user-ids"`
	GroupIDs []int64 `mapstructure:"group-ids" yaml:"group-ids"`
}

type BotAppsConfig struct {
	Name        string               `mapstructure:"name" yaml:"name"`
	Uri         string               `mapstructure:"uri" yaml:"uri"`
	AccessToken string               `mapstructure:"access-token" yaml:"access-token"`
	Private     MessageTypeConfig    `mapstructure:"private" yaml:"private"`
	Group       MessageTypeConfig    `mapstructure:"group" yaml:"group"`
	Message     MessageContentConfig `mapstructure:"message" yaml:"message"`
}

type MessageTypeConfig struct {
	Mode    string               `mapstructure:"mode" yaml:"mode"`
	Ids     []int64              `mapstructure:"ids" yaml:"ids"`
	Message MessageContentConfig `mapstructure:"message" yaml:"message"`
}

type MessageContentConfig struct {
	Mode          string                        `mapstructure:"mode" yaml:"mode"`
	Filters       []string                      `mapstructure:"filters" yaml:"filters"`
	Prefix        []string                      `mapstructure:"prefix" yaml:"prefix"`
	PrefixReplace string                        `mapstructure:"prefix-replace" yaml:"prefix-replace"`
	SpecificRules map[string]SpecificRuleConfig `mapstructure:"specific-rules" yaml:"specific-rules"`
}

type SpecificRuleConfig struct {
	Mode          string   `mapstructure:"mode" yaml:"mode,omitempty"`
	Filters       []string `mapstructure:"filters" yaml:"filters,omitempty"`
	Prefix        []string `mapstructure:"prefix" yaml:"prefix,omitempty"`
	AddFilters    []string `mapstructure:"add-filters" yaml:"add-filters,omitempty"`
	RemoveFilters []string `mapstructure:"remove-filters" yaml:"remove-filters,omitempty"`
	ClearFilters  bool     `mapstructure:"clear-filters" yaml:"clear-filters,omitempty"`
	AddPrefix     []string `mapstructure:"add-prefix" yaml:"add-prefix,omitempty"`
	RemovePrefix  []string `mapstructure:"remove-prefix" yaml:"remove-prefix,omitempty"`
	ClearPrefix   bool     `mapstructure:"clear-prefix" yaml:"clear-prefix,omitempty"`
	PrefixReplace *string  `mapstructure:"prefix-replace" yaml:"prefix-replace,omitempty"`
}

func LoadConfigVP(path string) error {
	configMutex.Lock()
	defer configMutex.Unlock()

	VP = viper.New()
	VP.SetConfigFile(path)
	VP.SetConfigType("yaml")
	if err := VP.ReadInConfig(); err != nil {
		return err
	}

	configPath = path

	VP.WatchConfig()
	VP.OnConfigChange(func(e fsnotify.Event) {
		log.Println("检测到配置文件变更:", e.Name)
		if err := VP.Unmarshal(&CONFIG); err != nil {
			log.Println("重新读取配置失败:", err)
			return
		}
		if err := CONFIG.Check(); err != nil {
			log.Println("配置文件校验失败:", err)
			return
		}
		if err := ReLoadFilters(); err != nil {
			log.Println("重新加载过滤器失败:", err)
		}
	})

	if err := VP.Unmarshal(&CONFIG); err != nil {
		return err
	}
	if err := CONFIG.Check(); err != nil {
		return errors.New("配置文件校验失败: " + err.Error())
	}
	return nil
}

func (c *Config) Check() error {
	if c.Server.Host == "" {
		return errors.New("server.host 不能为空")
	}
	if c.Server.Port == 0 {
		return errors.New("server.port 不能为空")
	}
	if c.Server.UserAgent == "" {
		return errors.New("server.user-agent 不能为空")
	}

	switch c.Server.Default.Private.Mode {
	case "", ON, OFF, WHITELIST, BLACKLIST:
	default:
		return errors.New("server.default.private.mode 配置错误，只能是 on、off、whitelist 或 blacklist")
	}

	switch c.Server.Default.Group.Mode {
	case "", ON, OFF, WHITELIST, BLACKLIST:
	default:
		return errors.New("server.default.group.mode 配置错误，只能是 on、off、whitelist 或 blacklist")
	}

	return nil
}

func checkSpecificRules(botName, scope string, rules map[string]SpecificRuleConfig) error {
	for idStr, rule := range rules {
		if _, err := strconv.ParseInt(idStr, 10, 64); err != nil {
			return fmt.Errorf("%s.%s.message.specific-rules.%s: ID 格式错误", botName, scope, idStr)
		}
		if rule.Mode != "" && rule.Mode != ON && rule.Mode != WHITELIST && rule.Mode != BLACKLIST {
			return fmt.Errorf("%s.%s.message.specific-rules.%s.mode 配置错误，只能是 on、whitelist 或 blacklist", botName, scope, idStr)
		}
	}
	return nil
}

func (bac *BotAppsConfig) Check() error {
	if bac.Name == "" {
		return errors.New("bot-apps.name 不能为空")
	}
	if bac.Uri == "" {
		return fmt.Errorf("%s.uri 不能为空", bac.Name)
	}

	switch bac.Private.Mode {
	case "", DEFAULT:
		bac.Private.Mode = CONFIG.Server.Default.Private.Mode
		bac.Private.Ids = CONFIG.Server.Default.Private.Ids
	case ON, OFF, WHITELIST, BLACKLIST:
	default:
		return fmt.Errorf("%s.private.mode 配置错误，只能是 on、off、whitelist 或 blacklist", bac.Name)
	}

	switch bac.Group.Mode {
	case "", DEFAULT:
		bac.Group.Mode = CONFIG.Server.Default.Group.Mode
		bac.Group.Ids = CONFIG.Server.Default.Group.Ids
	case ON, OFF, WHITELIST, BLACKLIST:
	default:
		return fmt.Errorf("%s.group.mode 配置错误，只能是 on、off、whitelist 或 blacklist", bac.Name)
	}

	switch bac.Message.Mode {
	case "", ON, WHITELIST, BLACKLIST:
	default:
		return fmt.Errorf("%s.message.mode 配置错误，只能是 on、whitelist 或 blacklist", bac.Name)
	}

	switch bac.Private.Message.Mode {
	case "", DEFAULT:
		bac.Private.Message = bac.Message
	case ON, WHITELIST, BLACKLIST:
	default:
		return fmt.Errorf("%s.private.message.mode 配置错误，只能是 default、on、whitelist 或 blacklist", bac.Name)
	}

	switch bac.Group.Message.Mode {
	case "", DEFAULT:
		bac.Group.Message = bac.Message
	case ON, WHITELIST, BLACKLIST:
	default:
		return fmt.Errorf("%s.group.message.mode 配置错误，只能是 default、on、whitelist 或 blacklist", bac.Name)
	}

	if bac.Private.Message.SpecificRules != nil {
		if err := checkSpecificRules(bac.Name, "private", bac.Private.Message.SpecificRules); err != nil {
			return err
		}
	}

	if bac.Group.Message.SpecificRules != nil {
		if err := checkSpecificRules(bac.Name, "group", bac.Group.Message.SpecificRules); err != nil {
			return err
		}
	}

	return nil
}
