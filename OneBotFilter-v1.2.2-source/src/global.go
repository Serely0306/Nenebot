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

// 过滤器模式
const (
	DEFAULT        = "default"
	ON             = "on"
	OFF            = "off"
	WHITELIST      = "whitelist"
	BLACKLIST      = "blacklist"
	DEFAULT_RULES  = "default"
	SPECIFIC_RULES = "specific"
)

// 消息类型
const (
	PRIVATE = "private"
	GROUP   = "group"
)

// 消息格式
// 消息的内容类型
const (
	MESSAGE_FORMAT_ARRAY  = "array"
	MESSAGE_FORMAT_STRING = "string"
	MESSAGE_TYPE_TEXT     = "text"
)

// 布尔值
var (
	TRUE  = true
	FALSE = false
)

// 配置文件相关
var (
	VP          *viper.Viper
	CONFIG      Config
	ALL_FILTERS []*Filter
	configMutex sync.RWMutex // 用于保护配置文件
	configPath  string       // 配置文件路径
)

type WsMsg struct {
	MsgType int
	MsgData []byte
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

// 重新加载所有过滤器
func ReLoadFilters() error {
	configMutex.RLock()
	defer configMutex.RUnlock()

	for _, botApp := range CONFIG.BotApps {
		//检查配置
		err := botApp.Check()
		if err != nil {
			log.Printf("bot %s 的配置文件校验失败：%v\n", botApp.Name, err)
			continue
		}
		for _, filter := range ALL_FILTERS {
			if filter.Name == botApp.Name {
				filter.Compile(botApp)
				log.Printf("已重新加载过滤器：%s\n", filter.String())
				break
			}
		}
	}
	log.Printf("重新加载过滤器，共有%d个\n", len(ALL_FILTERS))
	return nil
}

// UpdateBotAppConfig 更新BotApp的配置
func UpdateBotAppConfig(botName string, updateFunc func(*BotAppsConfig)) bool {
	configMutex.Lock()
	defer configMutex.Unlock()

	log.Printf("开始更新BotApp配置: %s\n", botName)

	found := false
	for i, botApp := range CONFIG.BotApps {
		if botApp.Name == botName {
			found = true

			// 保存原始配置用于比较
			oldPrivateIds := make([]int64, len(botApp.Private.Ids))
			copy(oldPrivateIds, botApp.Private.Ids)
			oldGroupIds := make([]int64, len(botApp.Group.Ids))
			copy(oldGroupIds, botApp.Group.Ids)

			// 调用更新函数
			updateFunc(&CONFIG.BotApps[i])

			// 检查配置是否真的改变了
			configChanged := false
			if !slices.Equal(oldPrivateIds, CONFIG.BotApps[i].Private.Ids) {
				configChanged = true
				log.Printf("私聊黑名单已改变: %v -> %v\n", oldPrivateIds, CONFIG.BotApps[i].Private.Ids)
			}
			if !slices.Equal(oldGroupIds, CONFIG.BotApps[i].Group.Ids) {
				configChanged = true
				log.Printf("群聊黑名单已改变: %v -> %v\n", oldGroupIds, CONFIG.BotApps[i].Group.Ids)
			}

			if !configChanged {
				log.Printf("配置未改变，跳过保存\n")
				return true
			}

			// 保存到文件
			log.Printf("开始保存配置到文件...\n")
			if err := saveConfigToFileLocked(); err != nil {
				log.Printf("保存配置失败: %v\n", err)
				return false
			}
			log.Printf("配置保存成功\n")

			// 重新编译过滤器
			for _, filter := range ALL_FILTERS {
				if filter.Name == botName {
					filter.Compile(CONFIG.BotApps[i])
					log.Printf("已重新编译过滤器: %s\n", filter.Name)
					break
				}
			}

			return true
		}
	}

	if !found {
		log.Printf("未找到名为 %s 的BotApp配置\n", botName)
	}

	return false
}

// saveConfigToFileLocked 将当前配置保存到配置文件（假设已经持有锁）
func saveConfigToFileLocked() error {
	if configPath == "" {
		log.Printf("配置文件路径为空\n")
		return errors.New("配置文件路径未设置")
	}

	log.Printf("正在保存配置文件到: %s\n", configPath)

	// 检查文件是否存在
	if _, err := os.Stat(configPath); err != nil {
		log.Printf("配置文件不存在或无法访问: %v\n", err)
		return fmt.Errorf("配置文件不存在或无法访问: %v", err)
	}

	// 备份原文件
	backupPath := configPath + ".bak"
	if err := copyFile(configPath, backupPath); err != nil {
		log.Printf("备份配置文件失败: %v\n", err)
	} else {
		log.Printf("已备份原配置文件到: %s\n", backupPath)
	}

	// 使用yaml库保存配置，保持格式
	data, err := yaml.Marshal(&CONFIG)
	if err != nil {
		log.Printf("序列化配置失败: %v\n", err)
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	log.Printf("配置数据大小: %d 字节\n", len(data))

	// 写入文件
	err = os.WriteFile(configPath, data, 0644)
	if err != nil {
		log.Printf("写入配置文件失败: %v\n", err)
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	// 验证文件是否写入成功
	fileInfo, err := os.Stat(configPath)
	if err != nil {
		log.Printf("无法获取文件信息: %v\n", err)
		return fmt.Errorf("无法获取文件信息: %v", err)
	}

	log.Printf("文件保存成功! 大小: %d 字节, 修改时间: %v\n",
		fileInfo.Size(), fileInfo.ModTime().Format("2006-01-02 15:04:05"))

	return nil
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, input, 0644)
}

// GetConfigPath 获取配置文件路径
func GetConfigPath() string {
	return configPath
}
