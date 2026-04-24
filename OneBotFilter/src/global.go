package onebotfilter

import (
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"sync"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

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

const (
	MESSAGE_FORMAT_ARRAY  = "array"
	MESSAGE_FORMAT_STRING = "string"
	MESSAGE_TYPE_TEXT     = "text"
)

var (
	TRUE  = true
	FALSE = false
)

var (
	VP          *viper.Viper
	CONFIG      Config
	ALL_FILTERS []*Filter
	configMutex sync.RWMutex
	configPath  string
	botInfoMu   sync.RWMutex
	botNickname string
)

type WsMsg struct {
	MsgType int
	MsgData []byte
}

func SetBotNickname(nickname string) {
	botInfoMu.Lock()
	defer botInfoMu.Unlock()
	botNickname = nickname
}

func GetBotNickname() string {
	botInfoMu.RLock()
	defer botInfoMu.RUnlock()
	return botNickname
}

func AddFilter(filter *Filter) {
	for _, f := range ALL_FILTERS {
		if f.Name == filter.Name {
			return
		}
	}
	ALL_FILTERS = append(ALL_FILTERS, filter)
}

func RemoveFilter(name string) {
	for i, f := range ALL_FILTERS {
		if f.Name == name {
			ALL_FILTERS = append(ALL_FILTERS[:i], ALL_FILTERS[i+1:]...)
			return
		}
	}
}

func ReLoadFilters() error {
	configMutex.RLock()
	defer configMutex.RUnlock()

	reloadFiltersLocked()
	return nil
}

func UpdateBotAppConfig(botName string, updateFunc func(*BotAppsConfig)) bool {
	configMutex.Lock()
	defer configMutex.Unlock()

	log.Printf("开始更新 BotApp 配置: %s\n", botName)

	for i, botApp := range CONFIG.BotApps {
		if botApp.Name != botName {
			continue
		}

		candidate, err := cloneBotAppConfig(botApp)
		if err != nil {
			log.Printf("复制 BotApp 配置失败: %v\n", err)
			return false
		}

		updateFunc(&candidate)

		if err := resolveBotAppMessagesLocked(&candidate); err != nil {
			log.Printf("更新后重建 BotApp 规则失败: %v\n", err)
			return false
		}

		if persistedBotAppEqual(botApp, candidate) {
			log.Printf("配置未变化，跳过保存\n")
			return true
		}

		CONFIG.BotApps[i] = candidate

		log.Printf("开始保存配置到文件...\n")
		markInternalConfigWrite()
		if err := saveConfigToFileLocked(); err != nil {
			log.Printf("保存配置失败: %v\n", err)
			return false
		}
		log.Printf("配置保存成功\n")

		for _, filter := range ALL_FILTERS {
			if filter.Name == botName {
				filter.Compile(candidate)
				log.Printf("已重新编译过滤器: %s\n", filter.Name)
				break
			}
		}

		return true
	}

	log.Printf("未找到名为 %s 的 BotApp 配置\n", botName)
	return false
}

func UpdateServerConfig(updateFunc func(*ServerConfig)) bool {
	configMutex.Lock()
	defer configMutex.Unlock()

	oldUserIDs := append([]int64(nil), CONFIG.Server.Blocked.UserIDs...)
	oldGroupIDs := append([]int64(nil), CONFIG.Server.Blocked.GroupIDs...)

	updateFunc(&CONFIG.Server)

	if slices.Equal(oldUserIDs, CONFIG.Server.Blocked.UserIDs) && slices.Equal(oldGroupIDs, CONFIG.Server.Blocked.GroupIDs) {
		return true
	}

	markInternalConfigWrite()
	if err := saveConfigToFileLocked(); err != nil {
		log.Printf("保存服务端配置失败: %v\n", err)
		return false
	}

	return true
}

func saveConfigToFileLocked() error {
	if configPath == "" {
		log.Printf("配置文件路径为空\n")
		return errors.New("配置文件路径未设置")
	}

	log.Printf("正在保存配置文件到 %s\n", configPath)

	if _, err := os.Stat(configPath); err != nil {
		log.Printf("配置文件不存在或无法访问: %v\n", err)
		return fmt.Errorf("配置文件不存在或无法访问: %v", err)
	}

	backupPath := configPath + ".bak"
	if err := copyFile(configPath, backupPath); err != nil {
		log.Printf("备份配置文件失败: %v\n", err)
	} else {
		log.Printf("已备份原配置文件到 %s\n", backupPath)
	}

	data, err := yaml.Marshal(&CONFIG)
	if err != nil {
		log.Printf("序列化配置失败: %v\n", err)
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	log.Printf("配置数据大小: %d 字节\n", len(data))

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		log.Printf("写入配置文件失败: %v\n", err)
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	fileInfo, err := os.Stat(configPath)
	if err != nil {
		log.Printf("无法获取文件信息: %v\n", err)
		return fmt.Errorf("无法获取文件信息: %v", err)
	}

	log.Printf("文件保存成功: 大小=%d 字节, 修改时间=%s\n",
		fileInfo.Size(), fileInfo.ModTime().Format("2006-01-02 15:04:05"))

	return nil
}

func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, input, 0644)
}

func GetConfigPath() string {
	return configPath
}
