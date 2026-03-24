package onebotfilter

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

type helpLink struct {
	Label string `mapstructure:"label" yaml:"label"`
	URL   string `mapstructure:"url" yaml:"url"`
}

type helpService struct {
	Name string `mapstructure:"name" yaml:"name"`
	Desc string `mapstructure:"desc" yaml:"desc"`
}

type helpSection struct {
	Title    string        `mapstructure:"title" yaml:"title"`
	Summary  []string      `mapstructure:"summary" yaml:"summary"`
	Commands []string      `mapstructure:"commands" yaml:"commands"`
	Services []helpService `mapstructure:"services" yaml:"services"`
	Links    []helpLink    `mapstructure:"links" yaml:"links"`
	Notes    []string      `mapstructure:"notes" yaml:"notes"`
}

type HelpFileConfig struct {
	Title      string        `mapstructure:"title" yaml:"title"`
	IntroLines []string      `mapstructure:"intro-lines" yaml:"intro-lines"`
	Sections   []helpSection `mapstructure:"sections" yaml:"sections"`
}

var (
	HELP_CONFIG     HelpFileConfig
	helpConfigMutex sync.RWMutex
)

func LoadHelpConfig(path string) error {
	helpConfigMutex.Lock()
	defer helpConfigMutex.Unlock()

	vp := viper.New()
	vp.SetConfigFile(path)
	vp.SetConfigType("yaml")
	if err := vp.ReadInConfig(); err != nil {
		return err
	}

	cfg := HelpFileConfig{}
	if err := vp.Unmarshal(&cfg); err != nil {
		return err
	}
	if err := cfg.Check(); err != nil {
		return err
	}

	HELP_CONFIG = cfg
	triggerHelpImageGeneration()

	vp.WatchConfig()
	vp.OnConfigChange(func(e fsnotify.Event) {
		next := HelpFileConfig{}
		if err := vp.Unmarshal(&next); err != nil {
			log.Printf("重新读取帮助配置失败: %v\n", err)
			return
		}
		if err := next.Check(); err != nil {
			log.Printf("帮助配置校验失败: %v\n", err)
			return
		}

		helpConfigMutex.Lock()
		HELP_CONFIG = next
		helpConfigMutex.Unlock()
		log.Printf("检测到帮助配置变更: %s\n", e.Name)

		triggerHelpImageGeneration()
	})

	return nil
}

func triggerHelpImageGeneration() {
	if !CONFIG.Server.Help.Generate {
		return
	}

	fontPath := CONFIG.Server.Help.Font
	if fontPath == "" {
		fontPath = "simkai.ttf"
	}
	imagePath := CONFIG.Server.Help.Image
	if imagePath == "" {
		imagePath = "help.png"
	}

	if cfgPath := strings.TrimSpace(GetConfigPath()); cfgPath != "" {
		dir := filepath.Dir(cfgPath)
		if !filepath.IsAbs(fontPath) {
			fontPath = filepath.Join(dir, fontPath)
		}
		if !filepath.IsAbs(imagePath) {
			imagePath = filepath.Join(dir, imagePath)
		}
	}

	if err := SaveHelpImage(HELP_CONFIG, fontPath, imagePath); err != nil {
		log.Printf("自动生成帮助图片失败: %v\n", err)
	} else {
		log.Printf("帮助图片已生成: %s\n", imagePath)
	}
}

func (c *HelpFileConfig) Check() error {
	if strings.TrimSpace(c.Title) == "" {
		return fmt.Errorf("help.title 不能为空")
	}
	if len(c.Sections) == 0 {
		return fmt.Errorf("help.sections 不能为空")
	}
	for idx, section := range c.Sections {
		if strings.TrimSpace(section.Title) == "" {
			return fmt.Errorf("help.sections[%d].title 不能为空", idx)
		}
	}
	return nil
}

func buildUnifiedHelpMessage() ([]map[string]interface{}, bool, error) {
	imageValue, ok, err := resolveUnifiedHelpImageBase64()
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	userID := resolveUnifiedHelpNodeUserID()
	nickname := getUnifiedHelpForwardNickname()
	title := getUnifiedHelpTitle()
	textContent := buildUnifiedHelpText()

	messages := []map[string]interface{}{
		{
			"type": "node",
			"data": map[string]interface{}{
				"user_id":  userID,
				"nickname": nickname,
				"content": []map[string]interface{}{
					{
						"type": "text",
						"data": map[string]interface{}{
							"text": title,
						},
					},
				},
			},
		},
		{
			"type": "node",
			"data": map[string]interface{}{
				"user_id":  userID,
				"nickname": nickname,
				"content": []map[string]interface{}{
					{
						"type": "image",
						"data": map[string]interface{}{
							"file": imageValue,
						},
					},
				},
			},
		},
	}

	if textContent != "" {
		messages = append(messages, map[string]interface{}{
			"type": "node",
			"data": map[string]interface{}{
				"user_id":  userID,
				"nickname": nickname,
				"content": []map[string]interface{}{
					{
						"type": "text",
						"data": map[string]interface{}{
							"text": textContent,
						},
					},
				},
			},
		})
	}

	return messages, true, nil
}

func resolveUnifiedHelpImageBase64() (string, bool, error) {
	_, checkPath, ok := resolveUnifiedHelpImagePath()
	if !ok {
		return "", false, nil
	}

	content, err := os.ReadFile(checkPath)
	if err != nil {
		return "", false, fmt.Errorf("读取帮助图片失败: %w", err)
	}

	return "base64://" + base64.StdEncoding.EncodeToString(content), true, nil
}

func resolveUnifiedHelpImagePath() (string, string, bool) {
	rawPath := strings.TrimSpace(CONFIG.Server.Help.Image)
	if rawPath == "" {
		return "", "", false
	}
	if filepath.IsAbs(rawPath) {
		return "", "", false
	}

	checkPath := filepath.Clean(filepath.FromSlash(rawPath))
	if cfgPath := strings.TrimSpace(GetConfigPath()); cfgPath != "" {
		checkPath = filepath.Join(filepath.Dir(cfgPath), checkPath)
	}

	info, err := os.Stat(checkPath)
	if err != nil || info.IsDir() {
		return "", "", false
	}

	return filepath.ToSlash(filepath.Clean(rawPath)), checkPath, true
}

func resolveUnifiedHelpNodeUserID() int64 {
	botID := strings.TrimSpace(CONFIG.Server.BotId)
	if botID == "" {
		return 0
	}
	value, err := strconv.ParseInt(botID, 10, 64)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func getUnifiedHelpTitle() string {
	helpConfigMutex.RLock()
	defer helpConfigMutex.RUnlock()
	if strings.TrimSpace(HELP_CONFIG.Title) == "" {
		return "统一帮助"
	}
	return HELP_CONFIG.Title
}

func getUnifiedHelpForwardNickname() string {
	if nickname := strings.TrimSpace(GetBotNickname()); nickname != "" {
		return nickname
	}
	value := strings.TrimSpace(CONFIG.Server.Help.ForwardNickname)
	if value != "" {
		return value
	}
	return getUnifiedHelpTitle()
}

func buildUnifiedHelpText() string {
	helpConfigMutex.RLock()
	cfg := HELP_CONFIG
	helpConfigMutex.RUnlock()

	var builder strings.Builder
	hasLinks := false

	for _, section := range cfg.Sections {
		if len(section.Links) == 0 {
			continue
		}
		if hasLinks {
			builder.WriteString("\n")
		}
		builder.WriteString(section.Title)
		builder.WriteString(":\n")
		for _, link := range section.Links {
			builder.WriteString("  ")
			builder.WriteString(strings.TrimSpace(link.Label))
			builder.WriteString(": ")
			builder.WriteString(strings.TrimSpace(link.URL))
			builder.WriteString("\n")
		}
		hasLinks = true
	}

	return strings.TrimSpace(builder.String())
}

func GenerateHelpImageFromConfig(outputPath string) error {
	helpConfigMutex.RLock()
	cfg := HELP_CONFIG
	helpConfigMutex.RUnlock()

	fontPath := CONFIG.Server.Help.Font
	if fontPath == "" {
		fontPath = "simkai.ttf"
	}

	if cfgPath := strings.TrimSpace(GetConfigPath()); cfgPath != "" {
		dir := filepath.Dir(cfgPath)
		if !filepath.IsAbs(fontPath) {
			fontPath = filepath.Join(dir, fontPath)
		}
		if outputPath != "" && !filepath.IsAbs(outputPath) {
			outputPath = filepath.Join(dir, outputPath)
		}
	}

	if outputPath == "" {
		outputPath = "help.png"
	}

	return SaveHelpImage(cfg, fontPath, outputPath)
}
