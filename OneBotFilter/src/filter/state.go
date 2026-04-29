package filter

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	"onebotfilter/src/core"
	"gopkg.in/yaml.v3"
)

var (
	registryMu       sync.RWMutex
	compiledFilters  []*Filter
	fallbackConfigMu sync.Mutex
	configPath       string
)

func init() {
	core.SetConfigResolver(ResolveConfig)
	core.SetConfigReloadHook(rebuildCompiledFiltersFromCoreConfig)
}

func snapshotCompiledFilters() []*Filter {
	registryMu.RLock()
	defer registryMu.RUnlock()

	cloned := make([]*Filter, len(compiledFilters))
	copy(cloned, compiledFilters)
	return cloned
}

func setCompiledFilters(filters []*Filter) {
	registryMu.Lock()
	defer registryMu.Unlock()

	compiledFilters = append([]*Filter(nil), filters...)
}

func getCompiledFilter(name string) *Filter {
	registryMu.RLock()
	defer registryMu.RUnlock()

	for _, filter := range compiledFilters {
		if filter.Name == name {
			return filter
		}
	}
	return nil
}

func rebuildCompiledFiltersFromCoreConfig() {
	syncConfigPathFromCore()
	rebuildCompiledFiltersFromConfig(&core.CONFIG)
}

func rebuildCompiledFiltersFromConfig(cfg *core.Config) {
	if cfg == nil {
		return
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	existing := make(map[string]*Filter, len(compiledFilters))
	for _, filter := range compiledFilters {
		existing[filter.Name] = filter
	}

	next := make([]*Filter, 0, len(cfg.BotApps))
	for i := range cfg.BotApps {
		botApp := cfg.BotApps[i]
		filter := existing[botApp.Name]
		if filter == nil {
			filter = &Filter{Name: botApp.Name}
		}
		filter.Compile(botApp)
		next = append(next, filter)
	}

	compiledFilters = next
}

func shouldRebuildCompiledFilters(cfg *core.Config) bool {
	if cfg == nil {
		return false
	}

	currentCount := len(core.CONFIG.BotApps)
	if currentCount == 0 {
		return true
	}
	return len(cfg.BotApps) == currentCount
}

func syncConfigPathFromCore() {
	if configPath != "" {
		return
	}
	if path := core.GetConfigPath(); path != "" {
		configPath = path
	}
}

func effectiveConfigPath() string {
	if configPath != "" {
		return configPath
	}
	return core.GetConfigPath()
}

func presetBaseDir() string {
	path := effectiveConfigPath()
	if path == "" {
		return filepath.Join("config")
	}

	configDir := filepath.Clean(filepath.Dir(path))
	if filepath.Base(configDir) == "filter" && filepath.Base(filepath.Dir(configDir)) == "config" {
		return filepath.Join(configDir, "presets")
	}
	return filepath.Join(configDir, "config")
}

func updateBotAppConfig(botName string, updateFunc func(*core.BotAppsConfig)) bool {
	if configPath == "" && core.GetConfigPath() != "" {
		return core.UpdateBotAppConfig(botName, updateFunc)
	}
	return updateBotAppConfigFallback(botName, updateFunc)
}

func updateServerConfig(updateFunc func(*core.ServerConfig)) bool {
	if configPath == "" && core.GetConfigPath() != "" {
		return core.UpdateServerConfig(updateFunc)
	}
	return updateServerConfigFallback(updateFunc)
}

func updateBotAppConfigFallback(botName string, updateFunc func(*core.BotAppsConfig)) bool {
	fallbackConfigMu.Lock()
	defer fallbackConfigMu.Unlock()

	for i, botApp := range core.CONFIG.BotApps {
		if botApp.Name != botName {
			continue
		}

		candidate, err := cloneBotAppConfig(botApp)
		if err != nil {
			return false
		}

		updateFunc(&candidate)

		tmpCfg := core.Config{
			Server:  core.CONFIG.Server,
			BotApps: []core.BotAppsConfig{candidate},
		}
		if err := resolvePresetMessagesLocked(&tmpCfg); err != nil {
			return false
		}
		if len(tmpCfg.BotApps) == 1 {
			candidate = tmpCfg.BotApps[0]
		}

		core.CONFIG.BotApps[i] = candidate
		if err := saveConfigToFile(); err != nil {
			return false
		}

		rebuildCompiledFiltersFromConfig(&core.CONFIG)
		return true
	}

	return false
}

func updateServerConfigFallback(updateFunc func(*core.ServerConfig)) bool {
	fallbackConfigMu.Lock()
	defer fallbackConfigMu.Unlock()

	oldUserIDs := append([]int64(nil), core.CONFIG.Server.Blocked.UserIDs...)
	oldGroupIDs := append([]int64(nil), core.CONFIG.Server.Blocked.GroupIDs...)

	updateFunc(&core.CONFIG.Server)
	if reflect.DeepEqual(oldUserIDs, core.CONFIG.Server.Blocked.UserIDs) && reflect.DeepEqual(oldGroupIDs, core.CONFIG.Server.Blocked.GroupIDs) {
		return true
	}

	if err := saveConfigToFile(); err != nil {
		return false
	}
	rebuildCompiledFiltersFromConfig(&core.CONFIG)
	return true
}

func saveConfigToFile() error {
	path := effectiveConfigPath()
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("配置文件不存在或无法访问: %v", err)
	}

	backupPath := path + ".bak"
	_ = copyFile(path, backupPath)

	data, err := yaml.Marshal(&core.CONFIG)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
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

func cloneBotAppConfig(botApp core.BotAppsConfig) (core.BotAppsConfig, error) {
	data, err := yaml.Marshal(botApp)
	if err != nil {
		return core.BotAppsConfig{}, err
	}

	var cloned core.BotAppsConfig
	if err := yaml.Unmarshal(data, &cloned); err != nil {
		return core.BotAppsConfig{}, err
	}
	return cloned, nil
}

