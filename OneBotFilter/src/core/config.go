package core

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig    `mapstructure:"server" yaml:"server"`
	BotApps []BotAppsConfig `mapstructure:"bot-apps" yaml:"bot-apps"`
}

type ServerConfig struct {
	Host        string            `mapstructure:"host" yaml:"host"`
	Port        uint              `mapstructure:"port" yaml:"port"`
	Suffix      string            `mapstructure:"suffix" yaml:"suffix"`
	BotID       string            `mapstructure:"bot-id" yaml:"bot-id"`
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
	IDs  []int64 `mapstructure:"ids" yaml:"ids"`
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
	Name        string                `mapstructure:"name" yaml:"name"`
	URI         string                `mapstructure:"uri" yaml:"uri"`
	AccessToken string                `mapstructure:"access-token" yaml:"access-token"`
	Private     MessageTypeConfig     `mapstructure:"private" yaml:"private"`
	Group       MessageTypeConfig     `mapstructure:"group" yaml:"group"`
	Message     *MessageContentConfig `mapstructure:"message" yaml:"message,omitempty"`
}

type MessageTypeConfig struct {
	Mode          string                `mapstructure:"mode" yaml:"mode"`
	IDs           []int64               `mapstructure:"ids" yaml:"ids"`
	Preset        string                `mapstructure:"preset" yaml:"preset"`
	Presets       map[string]string     `mapstructure:"presets" yaml:"presets"`
	InlineMessage *MessageContentConfig `mapstructure:"message" yaml:"message,omitempty"`
	Message       MessageContentConfig  `mapstructure:"-" yaml:"-"`
}

type MessageContentConfig struct {
	Mode           string                        `mapstructure:"mode" yaml:"mode"`
	Filters        []string                      `mapstructure:"filters" yaml:"filters"`
	Prefix         []string                      `mapstructure:"prefix" yaml:"prefix"`
	PrefixReplace  string                        `mapstructure:"prefix-replace" yaml:"prefix-replace"`
	PrefixFiltered bool                          `mapstructure:"prefix-filtered" yaml:"prefix-filtered"`
	SpecificRules  map[string]SpecificRuleConfig `mapstructure:"specific-rules" yaml:"specific-rules"`
}

type SpecificRuleConfig struct {
	Mode           string   `mapstructure:"mode" yaml:"mode,omitempty"`
	Filters        []string `mapstructure:"filters" yaml:"filters,omitempty"`
	Prefix         []string `mapstructure:"prefix" yaml:"prefix,omitempty"`
	AddFilters     []string `mapstructure:"add-filters" yaml:"add-filters,omitempty"`
	RemoveFilters  []string `mapstructure:"remove-filters" yaml:"remove-filters,omitempty"`
	ClearFilters   bool     `mapstructure:"clear-filters" yaml:"clear-filters,omitempty"`
	AddPrefix      []string `mapstructure:"add-prefix" yaml:"add-prefix,omitempty"`
	RemovePrefix   []string `mapstructure:"remove-prefix" yaml:"remove-prefix,omitempty"`
	ClearPrefix    bool     `mapstructure:"clear-prefix" yaml:"clear-prefix,omitempty"`
	PrefixReplace  *string  `mapstructure:"prefix-replace" yaml:"prefix-replace,omitempty"`
	PrefixFiltered *bool    `mapstructure:"prefix-filtered" yaml:"prefix-filtered,omitempty"`
}

type Paths struct {
	Root         string
	FilterConfig string
	HelpConfig   string
	StatsConfig  string
	HelpImage    string
	StatsDB      string
	LogDir       string
	LogFile      string
	FontFile     string
}

func DefaultPaths(root string) Paths {
	root = filepath.Clean(root)
	return Paths{
		Root:         root,
		FilterConfig: filepath.Join(root, "config", "filter", "config.yaml"),
		HelpConfig:   filepath.Join(root, "config", "help", "config.yaml"),
		StatsConfig:  filepath.Join(root, "config", "stats", "config.yaml"),
		HelpImage:    filepath.Join(root, "data", "help", "help.png"),
		StatsDB:      filepath.Join(root, "data", "stats", "stats.db"),
		LogDir:       filepath.Join(root, "data", "log"),
		LogFile:      filepath.Join(root, "data", "log", "onebotfilter.log"),
		FontFile:     filepath.Join("assets", "fonts", "simkai.ttf"),
	}
}

const (
	DEFAULT        = "default"
	ON             = "on"
	OFF            = "off"
	WHITELIST      = "whitelist"
	BLACKLIST      = "blacklist"
	DEFAULT_RULES  = "default"
	SPECIFIC_RULES = "specific"
)

const (
	PRIVATE = "private"
	GROUP   = "group"
)

var (
	presetWatcher          *fsnotify.Watcher
	skipConfigReloadEvents int32
)

func (cfg CommandAuthConfig) IsSuperUser(userID int64) bool {
	for _, value := range cfg.SuperUsers {
		if value == userID {
			return true
		}
	}
	return false
}

func LoadConfigVP(path string) error {
	configMutex.Lock()
	defer configMutex.Unlock()

	vp := viper.New()
	vp.SetConfigFile(path)
	vp.SetConfigType("yaml")
	configPath = path

	if err := loadConfigFromDiskLocked(vp); err != nil {
		return err
	}
	if err := restartPresetWatcherLocked(); err != nil {
		return err
	}

	vp.WatchConfig()
	vp.OnConfigChange(func(e fsnotify.Event) {
		if consumeConfigReloadSkip() {
			return
		}
		if err := reloadConfigLocked(vp); err != nil {
			return
		}
	})
	return nil
}

func loadConfigFromDiskLocked(vp *viper.Viper) error {
	if err := vp.ReadInConfig(); err != nil {
		return err
	}

	var cfg Config
	if err := vp.Unmarshal(&cfg); err != nil {
		return err
	}
	if configResolver != nil {
		if err := configResolver(&cfg); err != nil {
			return err
		}
	}
	if err := cfg.Check(); err != nil {
		return fmt.Errorf("配置文件校验失败: %w", err)
	}
	CONFIG = cfg
	return nil
}

func reloadConfigLocked(vp *viper.Viper) error {
	configMutex.Lock()
	if err := loadConfigFromDiskLocked(vp); err != nil {
		configMutex.Unlock()
		return err
	}
	if err := restartPresetWatcherLocked(); err != nil {
		configMutex.Unlock()
		return err
	}
	hook := configReloadHook
	configMutex.Unlock()
	if hook != nil {
		hook()
	}
	return nil
}

func restartPresetWatcherLocked() error {
	rootDir := filepath.Dir(configPath)
	targetDir := presetBaseDir(rootDir)

	if presetWatcher != nil {
		_ = presetWatcher.Close()
		presetWatcher = nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := watcher.Add(rootDir); err != nil {
		_ = watcher.Close()
		return err
	}
	if err := addWatchDirsRecursive(watcher, targetDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = watcher.Close()
		return err
	}

	presetWatcher = watcher
	go runPresetWatcher(watcher, targetDir)
	return nil
}

func runPresetWatcher(watcher *fsnotify.Watcher, targetDir string) {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			handlePresetWatchEvent(watcher, targetDir, event)
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func handlePresetWatchEvent(watcher *fsnotify.Watcher, targetDir string, event fsnotify.Event) {
	cleanPath := filepath.Clean(event.Name)
	if event.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(cleanPath); err == nil && info.IsDir() {
			if err := addWatchDirsRecursive(watcher, cleanPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return
			}
		}
	}
	if !isPresetConfigFile(cleanPath, targetDir) {
		return
	}
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
		return
	}

	vp := viper.New()
	vp.SetConfigFile(configPath)
	vp.SetConfigType("yaml")
	_ = reloadConfigLocked(vp)
}

func addWatchDirsRecursive(watcher *fsnotify.Watcher, targetDir string) error {
	info, err := os.Stat(targetDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("预设配置路径不是目录: %s", targetDir)
	}
	return filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		return watcher.Add(path)
	})
}

func isPresetConfigFile(path, targetDir string) bool {
	if targetDir == "" {
		return false
	}
	rel, err := filepath.Rel(targetDir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

func presetBaseDir(configDir string) string {
	configDir = filepath.Clean(configDir)
	if filepath.Base(configDir) == "filter" && filepath.Base(filepath.Dir(configDir)) == "config" {
		return filepath.Join(configDir, "presets")
	}
	return filepath.Join(configDir, "config")
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

	for i := range c.BotApps {
		if err := c.BotApps[i].checkWithDefaults(c.Server.Default); err != nil {
			return err
		}
	}
	return nil
}

func (bac *BotAppsConfig) Check() error {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return bac.checkWithDefaults(CONFIG.Server.Default)
}

func (bac *BotAppsConfig) checkWithDefaults(defaults DefaultConfig) error {
	if bac.Name == "" {
		return errors.New("bot-apps.name 不能为空")
	}
	if bac.URI == "" {
		return fmt.Errorf("%s.uri 不能为空", bac.Name)
	}

	switch bac.Private.Mode {
	case "", DEFAULT:
		bac.Private.Mode = defaults.Private.Mode
		bac.Private.IDs = defaults.Private.IDs
	case ON, OFF, WHITELIST, BLACKLIST:
	default:
		return fmt.Errorf("%s.private.mode 配置错误，只能是 on、off、whitelist 或 blacklist", bac.Name)
	}

	switch bac.Group.Mode {
	case "", DEFAULT:
		bac.Group.Mode = defaults.Group.Mode
		bac.Group.IDs = defaults.Group.IDs
	case ON, OFF, WHITELIST, BLACKLIST:
	default:
		return fmt.Errorf("%s.group.mode 配置错误，只能是 on、off、whitelist 或 blacklist", bac.Name)
	}

	if err := checkPresetMappings(bac.Name, "private", bac.Private.Presets); err != nil {
		return err
	}
	if err := checkPresetMappings(bac.Name, "group", bac.Group.Presets); err != nil {
		return err
	}
	if err := validateMessageMode(fmt.Sprintf("%s.private.message.mode", bac.Name), bac.Private.Message); err != nil {
		return err
	}
	if err := validateMessageMode(fmt.Sprintf("%s.group.message.mode", bac.Name), bac.Group.Message); err != nil {
		return err
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

func UpdateBotAppConfig(botName string, updateFunc func(*BotAppsConfig)) bool {
	configMutex.Lock()

	for i, botApp := range CONFIG.BotApps {
		if botApp.Name != botName {
			continue
		}
		candidate, err := cloneBotAppConfig(botApp)
		if err != nil {
			configMutex.Unlock()
			return false
		}
		updateFunc(&candidate)
		if configResolver != nil {
			tmpCfg := Config{Server: CONFIG.Server, BotApps: []BotAppsConfig{candidate}}
			if err := configResolver(&tmpCfg); err != nil {
				configMutex.Unlock()
				return false
			}
			if len(tmpCfg.BotApps) == 1 {
				candidate = tmpCfg.BotApps[0]
			}
		}
		if persistedBotAppEqual(botApp, candidate) {
			CONFIG.BotApps[i] = candidate
			hook := configReloadHook
			configMutex.Unlock()
			if hook != nil {
				hook()
			}
			return true
		}
		CONFIG.BotApps[i] = candidate
		markInternalConfigWrite()
		if err := saveConfigToFileLocked(); err != nil {
			configMutex.Unlock()
			return false
		}
		hook := configReloadHook
		configMutex.Unlock()
		if hook != nil {
			hook()
		}
		return true
	}
	configMutex.Unlock()
	return false
}

func UpdateServerConfig(updateFunc func(*ServerConfig)) bool {
	configMutex.Lock()

	oldUserIDs := append([]int64(nil), CONFIG.Server.Blocked.UserIDs...)
	oldGroupIDs := append([]int64(nil), CONFIG.Server.Blocked.GroupIDs...)

	updateFunc(&CONFIG.Server)
	if reflect.DeepEqual(oldUserIDs, CONFIG.Server.Blocked.UserIDs) && reflect.DeepEqual(oldGroupIDs, CONFIG.Server.Blocked.GroupIDs) {
		configMutex.Unlock()
		return true
	}
	markInternalConfigWrite()
	if err := saveConfigToFileLocked(); err != nil {
		configMutex.Unlock()
		return false
	}
	hook := configReloadHook
	configMutex.Unlock()
	if hook != nil {
		hook()
	}
	return true
}

func saveConfigToFileLocked() error {
	if configPath == "" {
		return errors.New("配置文件路径未设置")
	}
	if _, err := os.Stat(configPath); err != nil {
		return fmt.Errorf("配置文件不存在或无法访问: %v", err)
	}
	backupPath := configPath + ".bak"
	_ = copyFile(configPath, backupPath)

	data, err := yaml.Marshal(&CONFIG)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0o644)
}

func cloneBotAppConfig(botApp BotAppsConfig) (BotAppsConfig, error) {
	data, err := yaml.Marshal(botApp)
	if err != nil {
		return BotAppsConfig{}, err
	}
	var cloned BotAppsConfig
	if err := yaml.Unmarshal(data, &cloned); err != nil {
		return BotAppsConfig{}, err
	}
	return cloned, nil
}

func persistedBotAppSnapshot(botApp BotAppsConfig) BotAppsConfig {
	snapshot := botApp
	snapshot.Private.Message = MessageContentConfig{}
	snapshot.Group.Message = MessageContentConfig{}
	return snapshot
}

func persistedBotAppEqual(a, b BotAppsConfig) bool {
	return reflect.DeepEqual(persistedBotAppSnapshot(a), persistedBotAppSnapshot(b))
}

func markInternalConfigWrite() {
	atomic.StoreInt32(&skipConfigReloadEvents, 4)
}

func consumeConfigReloadSkip() bool {
	for {
		current := atomic.LoadInt32(&skipConfigReloadEvents)
		if current <= 0 {
			return false
		}
		if atomic.CompareAndSwapInt32(&skipConfigReloadEvents, current, current-1) {
			return true
		}
	}
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

func checkPresetMappings(botName, scope string, presets map[string]string) error {
	for idStr, presetName := range presets {
		if _, err := strconv.ParseInt(idStr, 10, 64); err != nil {
			return fmt.Errorf("%s.%s.presets.%s: ID 格式错误", botName, scope, idStr)
		}
		if strings.TrimSpace(presetName) == "" {
			return fmt.Errorf("%s.%s.presets.%s: 预设名不能为空", botName, scope, idStr)
		}
	}
	return nil
}

func validateMessageMode(scope string, message MessageContentConfig) error {
	switch message.Mode {
	case "", ON, WHITELIST, BLACKLIST:
		return nil
	default:
		return fmt.Errorf("%s 配置错误，只能是 on、whitelist 或 blacklist", scope)
	}
}
