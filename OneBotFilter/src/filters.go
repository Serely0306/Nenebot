package onebotfilter

import (
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"

	regexp "github.com/dlclark/regexp2"
)

type MessageTypeFilter struct {
	MessageTypeConfig
	Regexps       []*regexp.Regexp
	SpecificRules map[int64]*SpecificRuleFilter
}

type SpecificRuleFilter struct {
	Regexps       []*regexp.Regexp
	Prefix        []string
	PrefixReplace string
	Mode          string
}

type Filter struct {
	Name    string
	Private MessageTypeFilter
	Group   MessageTypeFilter
}

type MessageContentFilter struct {
	MessageContentConfig
	Regexps []*regexp.Regexp
}

func (f *Filter) Filter(onebotMessage *OneBotMessage) bool {
	if CONFIG.Server.Debug {
		log.Printf("%s：开始处理消息，类型=%s，群ID=%d，内容=%s\n",
			f.Name,
			onebotMessage.Partial.MessageType,
			onebotMessage.Partial.GroupId,
			onebotMessage.Partial.RawMessage,
		)
	}

	var usedFilter *MessageTypeFilter
	var targetID int64

	switch onebotMessage.Partial.MessageType {
	case PRIVATE:
		if onebotMessage.Partial.UserId == 0 {
			if CONFIG.Server.Debug {
				log.Printf("%s：私聊消息缺少 user_id，已阻止：%s\n", f.Name, onebotMessage.Partial.RawMessage)
			}
			return false
		}
		if f.Private.Filter(onebotMessage.Partial.UserId) {
			usedFilter = &f.Private
			targetID = onebotMessage.Partial.UserId
			break
		}
		if CONFIG.Server.Debug {
			log.Printf("%s：私聊 %d 未通过会话级过滤：%s\n", f.Name, onebotMessage.Partial.UserId, onebotMessage.Partial.RawMessage)
		}
		return false

	case GROUP:
		if onebotMessage.Partial.GroupId == 0 {
			if CONFIG.Server.Debug {
				log.Printf("%s：群消息缺少 group_id，已阻止：%s\n", f.Name, onebotMessage.Partial.RawMessage)
			}
			return false
		}
		if f.Group.Filter(onebotMessage.Partial.GroupId) {
			usedFilter = &f.Group
			targetID = onebotMessage.Partial.GroupId
			break
		}
		if CONFIG.Server.Debug {
			log.Printf("%s：群 %d 未通过会话级过滤：%s\n", f.Name, onebotMessage.Partial.GroupId, onebotMessage.Partial.RawMessage)
		}
		return false

	default:
		if CONFIG.Server.Debug {
			log.Printf("%s：message_type=%s，直接放行：%s\n", f.Name, onebotMessage.Partial.MessageType, onebotMessage.Partial.RawMessage)
		}
		return true
	}

	specificRule := usedFilter.getSpecificRule(targetID)
	ruleMode := usedFilter.getRuleMode(specificRule)

	if ruleMode != WHITELIST && ruleMode != BLACKLIST && ruleMode != "" && ruleMode != ON && ruleMode != OFF {
		log.Printf("%s：message.mode 配置异常，当前值=%s\n", f.Name, ruleMode)
		return false
	}

	if ruleMode == "" || ruleMode == ON {
		if CONFIG.Server.Debug {
			log.Printf("%s：消息直接通过：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return true
	}

	if ruleMode == OFF {
		if CONFIG.Server.Debug {
			log.Printf("%s：message.mode=off，已阻止：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return false
	}

	if usedFilter.prefixPassWithRule(onebotMessage, specificRule) {
		if CONFIG.Server.Debug {
			log.Printf("%s：命中前缀直通：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return true
	}

	matchResult := usedFilter.processFilterWithRule(f.Name, onebotMessage, specificRule)
	if matchResult != nil {
		return *matchResult
	}

	switch ruleMode {
	case WHITELIST:
		if CONFIG.Server.Debug {
			log.Printf("%s：未命中白名单规则，已阻止：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return false
	case BLACKLIST:
		if CONFIG.Server.Debug {
			log.Printf("%s：未命中黑名单规则，默认放行：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return true
	}

	log.Printf("%s：message.mode 配置异常\n", f.Name)
	return false
}

func (f *Filter) Compile(cfg BotAppsConfig) *Filter {
	f.Name = cfg.Name
	f.Private.Compile(cfg.Private)
	f.Group.Compile(cfg.Group)
	return f
}

func (f *MessageTypeFilter) Compile(cfg MessageTypeConfig) *MessageTypeFilter {
	f.MessageTypeConfig = cfg
	f.Regexps = compileRegexps(f.Message.Filters, "默认规则")
	f.SpecificRules = make(map[int64]*SpecificRuleFilter)

	for idStr, rule := range cfg.Message.SpecificRules {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		finalFilters := mergeStringRules(f.Message.Filters, rule.ClearFilters, rule.RemoveFilters, rule.AddFilters)
		if len(rule.Filters) > 0 {
			finalFilters = dedupeStrings(rule.Filters)
		}

		finalPrefix := mergeStringRules(f.Message.Prefix, rule.ClearPrefix, rule.RemovePrefix, rule.AddPrefix)
		if len(rule.Prefix) > 0 {
			finalPrefix = dedupeStrings(rule.Prefix)
		}

		finalPrefixReplace := f.Message.PrefixReplace
		if rule.PrefixReplace != nil {
			finalPrefixReplace = *rule.PrefixReplace
		}

		finalMode := f.Message.Mode
		if rule.Mode != "" {
			finalMode = rule.Mode
		}

		f.SpecificRules[id] = &SpecificRuleFilter{
			Regexps:       compileRegexps(finalFilters, fmt.Sprintf("特定规则 %d", id)),
			Prefix:        finalPrefix,
			PrefixReplace: finalPrefixReplace,
			Mode:          finalMode,
		}
	}

	return f
}

func (f *Filter) String() string {
	privateSpecificCount := 0
	if f.Private.SpecificRules != nil {
		privateSpecificCount = len(f.Private.SpecificRules)
	}

	groupSpecificCount := 0
	if f.Group.SpecificRules != nil {
		groupSpecificCount = len(f.Group.SpecificRules)
	}

	return fmt.Sprintf(`
	name: %s
	private: %s, ids: %v (包含%d个特定ID规则)
	group: %s, ids: %v (包含%d个特定ID规则)`,
		f.Name,
		f.Private.Mode, f.Private.Ids, privateSpecificCount,
		f.Group.Mode, f.Group.Ids, groupSpecificCount,
	)
}

func (f *MessageTypeFilter) Filter(id int64) bool {
	switch f.Mode {
	case "", ON:
		return true
	case OFF:
		return false
	case WHITELIST:
		return slices.Contains(f.Ids, id)
	case BLACKLIST:
		return !slices.Contains(f.Ids, id)
	default:
		return true
	}
}

func (f *MessageTypeFilter) prefixPassWithRule(onebotMessage *OneBotMessage, specificRule *SpecificRuleFilter) bool {
	prefixes := f.getPrefix(specificRule)
	prefixReplace := f.getPrefixReplace(specificRule)
	if len(prefixes) == 0 {
		return false
	}

	var textOld string
	var index int
	var message MessageContent
	var err error

	switch onebotMessage.Partial.MessageFormat {
	case MESSAGE_FORMAT_ARRAY:
		for index, message = range onebotMessage.Partial.MessageArray {
			if message.Type != MESSAGE_TYPE_TEXT {
				continue
			}
			textOld = strings.TrimSpace(message.Data["text"].(string))
			break
		}
	case MESSAGE_FORMAT_STRING:
		textOld = strings.TrimSpace(onebotMessage.Partial.MessageString)
	default:
		return false
	}

	if textOld == "" {
		return false
	}

	matchedPrefix := ""
	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(textOld, prefix) {
			matchedPrefix = prefix
			break
		}
	}

	if matchedPrefix == "" {
		return false
	}

	text := prefixReplace + textOld[len(matchedPrefix):]
	switch onebotMessage.Partial.MessageFormat {
	case MESSAGE_FORMAT_ARRAY:
		onebotMessage.Partial.MessageArray[index].Data["text"] = text
		if strings.TrimSpace(text) == "" {
			onebotMessage.Partial.MessageArray = append(
				onebotMessage.Partial.MessageArray[:index],
				onebotMessage.Partial.MessageArray[index+1:]...,
			)
		}
		onebotMessage.Intact["message"], err = json.Marshal(onebotMessage.Partial.MessageArray)
		if err != nil {
			log.Println("序列化改写后的数组消息失败:", err)
			return false
		}
	case MESSAGE_FORMAT_STRING:
		onebotMessage.Partial.MessageString = text
		onebotMessage.Intact["message"], err = json.Marshal(onebotMessage.Partial.MessageString)
		if err != nil {
			log.Println("序列化改写后的字符串消息失败:", err)
			return false
		}
	}

	onebotMessage.Partial.RawMessage = strings.Replace(onebotMessage.Partial.RawMessage, textOld, text, 1)
	onebotMessage.Intact["raw_message"], err = json.Marshal(onebotMessage.Partial.RawMessage)
	if err != nil {
		log.Println("序列化改写后的原始消息失败:", err)
		return false
	}

	return true
}

func (f *MessageTypeFilter) processFilterWithRule(filterName string, onebotMessage *OneBotMessage, specificRule *SpecificRuleFilter) *bool {
	ruleMode := f.getRuleMode(specificRule)

	var regexps []*regexp.Regexp
	if specificRule != nil {
		regexps = specificRule.Regexps
	} else {
		regexps = f.Regexps
	}

	if onebotMessage.Partial.MessageFormat == MESSAGE_FORMAT_ARRAY {
		for _, message := range onebotMessage.Partial.MessageArray {
			if message.Type != MESSAGE_TYPE_TEXT {
				continue
			}
			text := strings.TrimSpace(message.Data["text"].(string))
			for _, pattern := range regexps {
				if ok, err := pattern.MatchString(text); ok {
					switch ruleMode {
					case WHITELIST:
						log.Printf("%s：命中白名单消息：%s\n", filterName, onebotMessage.Partial.RawMessage)
						return &TRUE
					case BLACKLIST:
						log.Printf("%s：命中黑名单消息：%s\n", filterName, onebotMessage.Partial.RawMessage)
						return &FALSE
					}
				} else if err != nil {
					log.Printf("规则 %s 匹配消息时出错：%s\n", pattern.String(), onebotMessage.Partial.RawMessage)
				}
			}
		}
		return nil
	}

	text := strings.TrimSpace(onebotMessage.Partial.MessageString)
	for _, pattern := range regexps {
		if ok, err := pattern.MatchString(text); ok {
			switch ruleMode {
			case WHITELIST:
				log.Printf("%s：命中白名单消息：%s\n", filterName, onebotMessage.Partial.RawMessage)
				return &TRUE
			case BLACKLIST:
				log.Printf("%s：命中黑名单消息：%s\n", filterName, onebotMessage.Partial.RawMessage)
				return &FALSE
			}
		} else if err != nil {
			log.Printf("规则 %s 匹配消息时出错：%s\n", pattern.String(), onebotMessage.Partial.RawMessage)
		}
	}

	return nil
}

func (f *MessageTypeFilter) getSpecificRule(id int64) *SpecificRuleFilter {
	if f.SpecificRules == nil {
		return nil
	}
	return f.SpecificRules[id]
}

func (f *MessageTypeFilter) getRuleMode(specificRule *SpecificRuleFilter) string {
	if specificRule != nil {
		return specificRule.Mode
	}
	return f.Message.Mode
}

func (f *MessageTypeFilter) getPrefix(specificRule *SpecificRuleFilter) []string {
	if specificRule != nil {
		return specificRule.Prefix
	}
	return f.Message.Prefix
}

func (f *MessageTypeFilter) getPrefixReplace(specificRule *SpecificRuleFilter) string {
	if specificRule != nil {
		return specificRule.PrefixReplace
	}
	return f.Message.PrefixReplace
}

func compileRegexps(filters []string, scope string) []*regexp.Regexp {
	regexps := make([]*regexp.Regexp, 0, len(filters))
	for _, filter := range dedupeStrings(filters) {
		pattern, err := regexp.Compile(filter, regexp.None)
		if err != nil {
			log.Printf("编译%s正则失败：%s，错误：%v\n", scope, filter, err)
			continue
		}
		regexps = append(regexps, pattern)
	}
	return regexps
}

func mergeStringRules(parent []string, clear bool, remove []string, add []string) []string {
	merged := make([]string, 0, len(parent)+len(add))
	if !clear {
		merged = append(merged, parent...)
	}

	if len(remove) > 0 {
		removeSet := make(map[string]struct{}, len(remove))
		for _, item := range remove {
			removeSet[item] = struct{}{}
		}

		filtered := merged[:0]
		for _, item := range merged {
			if _, ok := removeSet[item]; ok {
				continue
			}
			filtered = append(filtered, item)
		}
		merged = filtered
	}

	merged = append(merged, add...)
	return dedupeStrings(merged)
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (f *Filter) AddToBlacklist(messageType string, id int64) bool {
	var targetFilter *MessageTypeFilter

	switch messageType {
	case PRIVATE:
		targetFilter = &f.Private
	case GROUP:
		targetFilter = &f.Group
	default:
		return false
	}

	if targetFilter.Message.Mode != BLACKLIST {
		targetFilter.Message.Mode = BLACKLIST
		log.Printf("%s：已将 %s 消息模式切换为 blacklist\n", f.Name, messageType)
	}

	if slices.Contains(targetFilter.Ids, id) {
		log.Printf("%s：%s ID %d 已在黑名单中\n", f.Name, messageType, id)
		return true
	}

	targetFilter.Ids = append(targetFilter.Ids, id)
	log.Printf("%s：已将 %s ID %d 加入黑名单，当前黑名单：%v\n",
		f.Name, messageType, id, targetFilter.Ids)

	return f.saveBlacklistToConfig(messageType)
}

func (f *Filter) RemoveFromBlacklist(messageType string, id int64) bool {
	var targetFilter *MessageTypeFilter

	switch messageType {
	case PRIVATE:
		targetFilter = &f.Private
	case GROUP:
		targetFilter = &f.Group
	default:
		return false
	}

	if targetFilter.Message.Mode != BLACKLIST {
		targetFilter.Message.Mode = BLACKLIST
		log.Printf("%s：已将 %s 消息模式切换为 blacklist\n", f.Name, messageType)
	}

	found := false
	newIDs := make([]int64, 0, len(targetFilter.Ids))
	for _, existingID := range targetFilter.Ids {
		if existingID == id {
			found = true
			continue
		}
		newIDs = append(newIDs, existingID)
	}

	if !found {
		log.Printf("%s：%s ID %d 不在黑名单中\n", f.Name, messageType, id)
		return false
	}

	targetFilter.Ids = newIDs
	log.Printf("%s：已将 %s ID %d 移出黑名单，当前黑名单：%v\n",
		f.Name, messageType, id, targetFilter.Ids)

	return f.saveBlacklistToConfig(messageType)
}

func (f *Filter) saveBlacklistToConfig(messageType string) bool {
	log.Printf("%s：开始保存黑名单配置，消息类型=%s，当前黑名单=%v\n",
		f.Name, messageType, f.GetBlacklist(messageType))

	success := UpdateBotAppConfig(f.Name, func(botApp *BotAppsConfig) {
		switch messageType {
		case PRIVATE:
			log.Printf("%s：更新私聊黑名单：%v -> %v\n", f.Name, botApp.Private.Ids, f.Private.Ids)
			botApp.Private.Ids = f.Private.Ids
			if botApp.Private.Mode != BLACKLIST {
				botApp.Private.Mode = BLACKLIST
				log.Printf("%s：已将私聊模式切换为 blacklist\n", f.Name)
			}
		case GROUP:
			log.Printf("%s：更新群聊黑名单：%v -> %v\n", f.Name, botApp.Group.Ids, f.Group.Ids)
			botApp.Group.Ids = f.Group.Ids
			if botApp.Group.Mode != BLACKLIST {
				botApp.Group.Mode = BLACKLIST
				log.Printf("%s：已将群聊模式切换为 blacklist\n", f.Name)
			}
		}
	})

	if success {
		log.Printf("%s：黑名单配置已成功保存到文件\n", f.Name)
	} else {
		log.Printf("%s：黑名单配置保存失败\n", f.Name)
	}

	return success
}

func (f *Filter) GetBlacklist(messageType string) []int64 {
	switch messageType {
	case PRIVATE:
		return f.Private.Ids
	case GROUP:
		return f.Group.Ids
	default:
		return []int64{}
	}
}
