package help

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"

	"onebotfilter/src/core"
	"gopkg.in/yaml.v3"
)

type Link struct {
	Label string `mapstructure:"label" yaml:"label"`
	URL   string `mapstructure:"url" yaml:"url"`
}

type Service struct {
	Name string `mapstructure:"name" yaml:"name"`
	Desc string `mapstructure:"desc" yaml:"desc"`
}

type Section struct {
	Title    string    `mapstructure:"title" yaml:"title"`
	Summary  []string  `mapstructure:"summary" yaml:"summary"`
	Commands []string  `mapstructure:"commands" yaml:"commands"`
	Services []Service `mapstructure:"services" yaml:"services"`
	Links    []Link    `mapstructure:"links" yaml:"links"`
	Notes    []string  `mapstructure:"notes" yaml:"notes"`
}

type Config struct {
	Title      string    `mapstructure:"title" yaml:"title"`
	IntroLines []string  `mapstructure:"intro-lines" yaml:"intro-lines"`
	Sections   []Section `mapstructure:"sections" yaml:"sections"`
}

type Settings struct {
	Enabled         bool
	GenerateImage   bool
	ForwardNickname string
	BotID           string
	BotNickname     string
}

type Module struct {
	cfg      Config
	paths    core.Paths
	settings Settings
}

func NewModule(cfg Config, paths core.Paths, settings Settings) *Module {
	return &Module{cfg: cfg, paths: paths, settings: settings}
}

func (m *Module) Config() Config {
	return m.cfg
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.Check(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Load(path string, paths core.Paths, settings Settings) (*Module, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	mod := NewModule(cfg, paths, settings)
	if settings.GenerateImage {
		if err := mod.GenerateImage(); err != nil {
			return nil, err
		}
	}
	return mod, nil
}

func (m *Module) TryHandle(msg *core.OneBotMessage) (bool, map[string]any) {
	text := extractMessageText(msg)
	switch text {
	case "help", "/help", "帮助", "/帮助":
		forward, enabled, err := m.BuildUnifiedHelpMessage()
		if err != nil {
			return true, buildReply(msg, fmt.Sprintf("统一帮助生成失败: %v", err))
		}
		if !enabled {
			return false, nil
		}
		return true, buildForwardReply(msg, forward)
	default:
		return false, nil
	}
}

func (m *Module) BuildUnifiedHelpMessage() ([]map[string]any, bool, error) {
	imageValue, ok, err := m.resolveUnifiedHelpImageBase64()
	if err != nil {
		return nil, false, err
	}
	if !ok || !m.settings.Enabled {
		return nil, false, nil
	}

	userID := m.resolveUnifiedHelpNodeUserID()
	nickname := m.getUnifiedHelpForwardNickname()
	title := m.getUnifiedHelpTitle()
	textContent := m.buildUnifiedHelpText()

	messages := []map[string]any{
		{
			"type": "node",
			"data": map[string]any{
				"user_id":  userID,
				"nickname": nickname,
				"content": []map[string]any{
					{
						"type": "text",
						"data": map[string]any{
							"text": title,
						},
					},
				},
			},
		},
		{
			"type": "node",
			"data": map[string]any{
				"user_id":  userID,
				"nickname": nickname,
				"content": []map[string]any{
					{
						"type": "image",
						"data": map[string]any{
							"file": imageValue,
						},
					},
				},
			},
		},
	}

	if textContent != "" {
		messages = append(messages, map[string]any{
			"type": "node",
			"data": map[string]any{
				"user_id":  userID,
				"nickname": nickname,
				"content": []map[string]any{
					{
						"type": "text",
						"data": map[string]any{
							"text": textContent,
						},
					},
				},
			},
		})
	}

	return messages, true, nil
}

func (c Config) Check() error {
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

func (m *Module) resolveUnifiedHelpImageBase64() (string, bool, error) {
	if strings.TrimSpace(m.paths.HelpImage) == "" {
		return "", false, nil
	}

	content, err := os.ReadFile(m.paths.HelpImage)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("读取帮助图片失败: %w", err)
	}

	return "base64://" + base64.StdEncoding.EncodeToString(content), true, nil
}

func (m *Module) resolveUnifiedHelpNodeUserID() int64 {
	botID := strings.TrimSpace(m.settings.BotID)
	if botID == "" {
		return 0
	}
	value, err := strconv.ParseInt(botID, 10, 64)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func (m *Module) getUnifiedHelpTitle() string {
	if strings.TrimSpace(m.cfg.Title) == "" {
		return "统一帮助"
	}
	return m.cfg.Title
}

func (m *Module) getUnifiedHelpForwardNickname() string {
	if nickname := strings.TrimSpace(m.settings.BotNickname); nickname != "" {
		return nickname
	}
	if value := strings.TrimSpace(m.settings.ForwardNickname); value != "" {
		return value
	}
	return m.getUnifiedHelpTitle()
}

func (m *Module) buildUnifiedHelpText() string {
	var builder strings.Builder
	hasLinks := false

	for _, section := range m.cfg.Sections {
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

func extractMessageText(msg *core.OneBotMessage) string {
	if msg == nil {
		return ""
	}
	if strings.TrimSpace(msg.Partial.MessageString) != "" {
		return strings.TrimSpace(msg.Partial.MessageString)
	}
	if strings.TrimSpace(msg.Partial.RawMessage) != "" {
		return strings.TrimSpace(msg.Partial.RawMessage)
	}
	return ""
}

func buildReply(msg *core.OneBotMessage, text string) map[string]any {
	params := map[string]any{"message": text}
	switch msg.Partial.MessageType {
	case "group":
		params["group_id"] = msg.Partial.GroupID
		return map[string]any{"action": "send_group_msg", "params": params}
	default:
		params["user_id"] = core.GetMessageUserID(msg)
		return map[string]any{"action": "send_private_msg", "params": params}
	}
}

func buildForwardReply(msg *core.OneBotMessage, messages []map[string]any) map[string]any {
	params := map[string]any{"messages": messages}
	switch msg.Partial.MessageType {
	case "group":
		params["group_id"] = msg.Partial.GroupID
		return map[string]any{"action": "send_group_forward_msg", "params": params}
	default:
		params["user_id"] = core.GetMessageUserID(msg)
		return map[string]any{"action": "send_private_forward_msg", "params": params}
	}
}
