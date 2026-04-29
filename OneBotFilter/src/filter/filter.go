package filter

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"onebotfilter/src/core"
	"gopkg.in/yaml.v3"
)

const (
	DEFAULT        = core.DEFAULT
	ON             = core.ON
	OFF            = core.OFF
	WHITELIST      = core.WHITELIST
	BLACKLIST      = core.BLACKLIST
	DEFAULT_RULES  = core.DEFAULT_RULES
	SPECIFIC_RULES = core.SPECIFIC_RULES

	PRIVATE = core.PRIVATE
	GROUP   = core.GROUP

	MESSAGE_FORMAT_ARRAY  = core.MessageFormatArray
	MESSAGE_FORMAT_STRING = core.MessageFormatString
	MESSAGE_TYPE_TEXT     = core.MessageTypeText
)

type (
	Config                   = core.Config
	ServerConfig             = core.ServerConfig
	DefaultConfig            = core.DefaultConfig
	DefaultMessageTypeConfig = core.DefaultMessageTypeConfig
	CommandAuthConfig        = core.CommandAuthConfig
	BlockedConfig            = core.BlockedConfig
	BotAppsConfig            = core.BotAppsConfig
	MessageTypeConfig        = core.MessageTypeConfig
	MessageContentConfig     = core.MessageContentConfig
	SpecificRuleConfig       = core.SpecificRuleConfig
	OneBotMessage            = core.OneBotMessage
	OneBotMessagePartial     = core.OneBotMessagePartial
	OneBotSender             = core.OneBotSender
	MessageContent           = core.MessageContent
)

type Module struct{}

func NewModule(_ any) *Module {
	return &Module{}
}

func (m *Module) Reload() error {
	return ResolveConfig(&core.CONFIG)
}

func (m *Module) FilterBotMessage(botName string, msg *core.OneBotMessage) bool {
	filter := getCompiledFilter(botName)
	if filter == nil {
		return true
	}
	return filter.Filter(msg)
}

func ResolveConfig(cfg *core.Config) error {
	if cfg == nil {
		return nil
	}

	syncConfigPathFromCore()
	if err := resolvePresetMessagesLocked(cfg); err != nil {
		return err
	}
	if shouldRebuildCompiledFilters(cfg) {
		rebuildCompiledFiltersFromConfig(cfg)
	}
	return nil
}

func resolvePresetMessagesLocked(cfg *Config) error {
	presetCache := make(map[string]MessageContentConfig)
	for i := range cfg.BotApps {
		botApp := &cfg.BotApps[i]
		if err := resolveBotAppMessagesLockedWithCache(botApp, presetCache); err != nil {
			return err
		}
	}
	return nil
}

func resolveBotAppMessagesLocked(botApp *BotAppsConfig) error {
	return resolveBotAppMessagesLockedWithCache(botApp, make(map[string]MessageContentConfig))
}

func resolveBotAppMessagesLockedWithCache(botApp *BotAppsConfig, presetCache map[string]MessageContentConfig) error {
	baseDir := presetBaseDir()
	if err := resolveMessageTypePresetsLocked(baseDir, presetCache, botApp, PRIVATE, &botApp.Private); err != nil {
		return err
	}
	if err := resolveMessageTypePresetsLocked(baseDir, presetCache, botApp, GROUP, &botApp.Group); err != nil {
		return err
	}
	return nil
}

func resolveMessageTypePresetsLocked(baseDir string, presetCache map[string]MessageContentConfig, botApp *BotAppsConfig, scope string, mt *MessageTypeConfig) error {
	baseMessage, err := resolveBaseMessageContentLocked(baseDir, presetCache, botApp, scope, mt)
	if err != nil {
		return err
	}

	specificRules := cloneSpecificRules(baseMessage.SpecificRules)
	for idStr, presetName := range mt.Presets {
		if _, err := strconv.ParseInt(idStr, 10, 64); err != nil {
			return fmt.Errorf("%s.%s.presets.%s: ID 格式错误", botApp.Name, scope, idStr)
		}
		presetMessage, err := loadPresetMessageLocked(baseDir, presetCache, botApp.Name, presetName)
		if err != nil {
			return fmt.Errorf("%s.%s.presets.%s: %w", botApp.Name, scope, idStr, err)
		}
		specificRules[idStr] = presetMessageToSpecificRule(presetMessage)
	}

	baseMessage.SpecificRules = specificRules
	mt.Message = baseMessage
	return nil
}

func resolveBaseMessageContentLocked(baseDir string, presetCache map[string]MessageContentConfig, botApp *BotAppsConfig, scope string, mt *MessageTypeConfig) (MessageContentConfig, error) {
	if mt.Preset != "" && mt.InlineMessage != nil {
		return MessageContentConfig{}, fmt.Errorf("%s.%s 同时配置了 preset 和 message，语义冲突", botApp.Name, scope)
	}

	presetName := strings.TrimSpace(mt.Preset)
	if scope == PRIVATE && presetName == "" && mt.InlineMessage == nil && botApp.Message == nil {
		if _, err := loadPresetMessageLocked(baseDir, presetCache, botApp.Name, "0"); err == nil {
			presetName = "0"
		}
	}

	switch {
	case presetName != "":
		return loadPresetMessageLocked(baseDir, presetCache, botApp.Name, presetName)
	case mt.InlineMessage != nil:
		return cloneMessageContentConfig(*mt.InlineMessage), nil
	case botApp.Message != nil:
		return cloneMessageContentConfig(*botApp.Message), nil
	default:
		return MessageContentConfig{}, nil
	}
}

func loadPresetMessageLocked(baseDir string, presetCache map[string]MessageContentConfig, botName, presetName string) (MessageContentConfig, error) {
	presetName = strings.TrimSpace(presetName)
	if presetName == "" {
		return MessageContentConfig{}, errors.New("预设名不能为空")
	}

	fileName := presetName
	if filepath.Ext(fileName) == "" {
		fileName += ".yaml"
	}

	fullPath := filepath.Join(baseDir, botName, fileName)
	if cached, ok := presetCache[fullPath]; ok {
		return cloneMessageContentConfig(cached), nil
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return MessageContentConfig{}, fmt.Errorf("读取预设失败: %w", err)
	}

	var message MessageContentConfig
	if err := yaml.Unmarshal(data, &message); err != nil {
		return MessageContentConfig{}, fmt.Errorf("解析预设失败: %w", err)
	}
	if len(message.SpecificRules) > 0 {
		return MessageContentConfig{}, errors.New("预设文件不允许包含 specific-rules，请在主配置中通过 presets 映射")
	}

	message.SpecificRules = nil
	presetCache[fullPath] = cloneMessageContentConfig(message)
	return cloneMessageContentConfig(message), nil
}

func presetMessageToSpecificRule(message MessageContentConfig) SpecificRuleConfig {
	prefixReplace := message.PrefixReplace
	prefixFiltered := message.PrefixFiltered

	rule := SpecificRuleConfig{
		Filters:        append([]string(nil), message.Filters...),
		Prefix:         append([]string(nil), message.Prefix...),
		ClearFilters:   true,
		ClearPrefix:    true,
		AddFilters:     append([]string(nil), message.Filters...),
		AddPrefix:      append([]string(nil), message.Prefix...),
		PrefixReplace:  &prefixReplace,
		PrefixFiltered: &prefixFiltered,
	}
	if message.Mode != "" {
		rule.Mode = message.Mode
	}
	return rule
}

func cloneMessageContentConfig(message MessageContentConfig) MessageContentConfig {
	return MessageContentConfig{
		Mode:           message.Mode,
		Filters:        append([]string(nil), message.Filters...),
		Prefix:         append([]string(nil), message.Prefix...),
		PrefixReplace:  message.PrefixReplace,
		PrefixFiltered: message.PrefixFiltered,
		SpecificRules:  cloneSpecificRules(message.SpecificRules),
	}
}

func cloneSpecificRules(rules map[string]SpecificRuleConfig) map[string]SpecificRuleConfig {
	if len(rules) == 0 {
		return map[string]SpecificRuleConfig{}
	}

	cloned := make(map[string]SpecificRuleConfig, len(rules))
	for key, rule := range rules {
		copiedRule := SpecificRuleConfig{
			Mode:          rule.Mode,
			Filters:       append([]string(nil), rule.Filters...),
			Prefix:        append([]string(nil), rule.Prefix...),
			AddFilters:    append([]string(nil), rule.AddFilters...),
			RemoveFilters: append([]string(nil), rule.RemoveFilters...),
			ClearFilters:  rule.ClearFilters,
			AddPrefix:     append([]string(nil), rule.AddPrefix...),
			RemovePrefix:  append([]string(nil), rule.RemovePrefix...),
			ClearPrefix:   rule.ClearPrefix,
		}
		if rule.PrefixReplace != nil {
			value := *rule.PrefixReplace
			copiedRule.PrefixReplace = &value
		}
		if rule.PrefixFiltered != nil {
			value := *rule.PrefixFiltered
			copiedRule.PrefixFiltered = &value
		}
		cloned[key] = copiedRule
	}
	return cloned
}

