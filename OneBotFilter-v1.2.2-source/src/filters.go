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
	Regexps       []*regexp.Regexp              // 默认正则表达式
	SpecificRules map[int64]*SpecificRuleFilter // 新增：特定ID的规则
}

// 新增：特定ID规则过滤器
type SpecificRuleFilter struct {
	Regexps       []*regexp.Regexp
	Prefix        []string
	PrefixReplace string
	Mode          string
}

// Filter结构保持不变
type Filter struct {
	Name    string
	Private MessageTypeFilter
	Group   MessageTypeFilter
}

// 已弃用的通用message过滤器（保持但不再使用）
type MessageContentFilter struct {
	MessageContentConfig
	Regexps []*regexp.Regexp //编译后的正则表达式
}

// handleCommand 处理控制命令（如/禁用、/启用）
func (f *Filter) handleCommand(onebotMessage *OneBotMessage) bool {
	// 只处理群消息
	if onebotMessage.Partial.MessageType != GROUP {
		return false
	}

	// 解析消息内容
	var messageText string
	switch onebotMessage.Partial.MessageFormat {
	case MESSAGE_FORMAT_ARRAY:
		for _, msg := range onebotMessage.Partial.MessageArray {
			if msg.Type == MESSAGE_TYPE_TEXT {
				messageText = strings.TrimSpace(msg.Data["text"].(string))
				break
			}
		}
	case MESSAGE_FORMAT_STRING:
		messageText = strings.TrimSpace(onebotMessage.Partial.MessageString)
	default:
		return false
	}

	if messageText == "" {
		return false
	}

	// 检查是否为控制命令
	parts := strings.Fields(messageText)
	if len(parts) != 2 {
		return false
	}

	command := parts[0]
	botName := parts[1]

	// 只处理针对当前过滤器的命令
	if botName != f.Name {
		return false
	}

	// 获取群号
	groupId := onebotMessage.Partial.GroupId
	if groupId == 0 {
		return false
	}

	// 执行命令
	var success bool
	var responseMsg string

	switch command {
	case "/禁用":
		success = f.AddToBlacklist(GROUP, groupId)
		if success {
			responseMsg = fmt.Sprintf("已在此群禁用 %s 机器人", f.Name)
		} else {
			responseMsg = fmt.Sprintf("禁用 %s 机器人失败", f.Name)
		}
	case "/启用":
		success = f.RemoveFromBlacklist(GROUP, groupId)
		if success {
			responseMsg = fmt.Sprintf("已在此群启用 %s 机器人", f.Name)
		} else {
			responseMsg = fmt.Sprintf("%s 机器人已在此群启用或启用失败", f.Name)
		}
	default:
		return false
	}

	// 创建命令响应消息
	response := map[string]interface{}{
		"action": "send_group_msg",
		"params": map[string]interface{}{
			"group_id": groupId,
			"message":  responseMsg,
		},
	}

	// 标记为命令响应
	onebotMessage.Partial.IsCommandResponse = true
	onebotMessage.Partial.CommandResponse = response

	if CONFIG.Server.Debug {
		log.Printf("%s：已处理命令，响应消息：%s\n", f.Name, responseMsg)
	}

	return true
}

func (f *Filter) Filter(onebotMessage *OneBotMessage) bool {
	// 先尝试处理命令
	commandHandled := f.handleCommand(onebotMessage)

	// 如果是命令且已处理，返回 false 阻止原始命令消息发送给bot应用端
	if commandHandled {
		if CONFIG.Server.Debug {
			log.Printf("%s：命令已处理，阻止原始消息传递\n", f.Name)
		}
		return false
	}

	// 记录开始处理的消息
	if CONFIG.Server.Debug {
		log.Printf("%s：处理消息，类型：%s，群ID：%d，内容：%s\n",
			f.Name, onebotMessage.Partial.MessageType,
			onebotMessage.Partial.GroupId, onebotMessage.Partial.RawMessage)
	}

	var usedFilter *MessageTypeFilter
	var targetID int64

	switch onebotMessage.Partial.MessageType {
	case PRIVATE:
		if onebotMessage.Partial.UserId == 0 {
			if CONFIG.Server.Debug {
				log.Printf("%s：没有user_id字段的private消息，过滤器被阻止：%s\n", f.Name, onebotMessage.Partial.RawMessage)
			}
			return false
		}
		if f.Private.Filter(onebotMessage.Partial.UserId) {
			usedFilter = &f.Private
			targetID = onebotMessage.Partial.UserId
			break
		}
		if CONFIG.Server.Debug {
			log.Printf("%s：%d的私聊不通过：%s\n", f.Name, onebotMessage.Partial.UserId, onebotMessage.Partial.RawMessage)
		}
		return false
	case GROUP:
		if onebotMessage.Partial.GroupId == 0 {
			if CONFIG.Server.Debug {
				log.Printf("%s：没有group_id字段的group消息，直接阻止：%s\n", f.Name, onebotMessage.Partial.RawMessage)
			}
			return false
		}
		if f.Group.Filter(onebotMessage.Partial.GroupId) {
			usedFilter = &f.Group
			targetID = onebotMessage.Partial.GroupId
			break
		}
		if CONFIG.Server.Debug {
			log.Printf("%s：%d的群消息不通过：%s\n", f.Name, onebotMessage.Partial.GroupId, onebotMessage.Partial.RawMessage)
		}
		return false
	default:
		if CONFIG.Server.Debug {
			log.Printf("%s：message_type=%s的消息，直接放行：%s\n", f.Name, onebotMessage.Partial.MessageType, onebotMessage.Partial.RawMessage)
		}
		return true
	}

	// 获取特定规则（如果有）
	specificRule := usedFilter.getSpecificRule(targetID)

	// 获取要使用的模式
	ruleMode := usedFilter.getRuleMode(specificRule)

	// 检查模式是否有效
	if ruleMode != WHITELIST && ruleMode != BLACKLIST && ruleMode != "" && ruleMode != ON && ruleMode != OFF {
		log.Printf("%s的message.mode配置异常，必须为whitelist或blacklist，当前为: %s\n", f.Name, ruleMode)
		return false
	}

	// 若模式为空或为ON（表示放行），直接通过
	if ruleMode == "" || ruleMode == ON {
		if CONFIG.Server.Debug {
			log.Printf("%s：直接通过的消息：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return true
	}

	// 若模式为OFF，直接阻止
	if ruleMode == OFF {
		if CONFIG.Server.Debug {
			log.Printf("%s：模式为OFF，阻止消息：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return false
	}

	// 前缀通过检查（使用特定规则或默认规则的前缀）
	if usedFilter.prefixPassWithRule(onebotMessage, specificRule) {
		log.Printf("%s：前缀通过的消息：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		return true
	}

	// 正则匹配
	matchResult := usedFilter.processFilterWithRule(f.Name, onebotMessage, specificRule)
	if matchResult != nil {
		return *matchResult
	}

	// 依据模式决定默认行为
	switch ruleMode {
	case WHITELIST:
		if CONFIG.Server.Debug {
			log.Printf("%s：不在白名单中的消息：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return false
	case BLACKLIST:
		if CONFIG.Server.Debug {
			log.Printf("%s：不在黑名单中的消息（默认允许）：%s\n", f.Name, onebotMessage.Partial.RawMessage)
		}
		return true
	}

	log.Printf("%s的message.mode配置异常，必须为whitelist或blacklist\n", f.Name)
	return false
}

// Compile 编译过滤器（从配置生成过滤器）
func (f *Filter) Compile(cfg BotAppsConfig) *Filter {
	f.Name = cfg.Name
	f.Private.Compile(cfg.Private)
	f.Group.Compile(cfg.Group)
	return f
}

// 修改MessageTypeFilter的Compile方法
func (f *MessageTypeFilter) Compile(cfg MessageTypeConfig) *MessageTypeFilter {
	f.MessageTypeConfig = cfg
	newFilters := []*regexp.Regexp{}

	// 编译默认规则的正则表达式
	for _, filter := range f.Message.Filters {
		pattern, err := regexp.Compile(filter, regexp.None)
		if err != nil {
			log.Printf("编译正则表达式：%s，出错：%v\n", filter, err)
			continue
		}
		newFilters = append(newFilters, pattern)
	}
	f.Regexps = newFilters

	// 初始化特定规则映射
	f.SpecificRules = make(map[int64]*SpecificRuleFilter)

	// 编译特定ID的规则
	if cfg.Message.SpecificRules != nil {
		for idStr, rule := range cfg.Message.SpecificRules {
			id, _ := strconv.ParseInt(idStr, 10, 64)

			// 编译正则表达式
			regexps := []*regexp.Regexp{}
			for _, filter := range rule.Filters {
				pattern, err := regexp.Compile(filter, regexp.None)
				if err != nil {
					log.Printf("编译特定规则正则表达式：%s，出错：%v\n", filter, err)
					continue
				}
				regexps = append(regexps, pattern)
			}

			// 存储到过滤器
			f.SpecificRules[id] = &SpecificRuleFilter{
				Regexps:       regexps,
				Prefix:        rule.Prefix,
				PrefixReplace: rule.PrefixReplace,
				Mode:          rule.Mode,
			}
		}
	}

	return f
}

func (f *Filter) String() string {
	// 简化String输出，避免过长
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
	private: %s , ids: %v (包含%d个特定ID规则)
	group: %s , ids: %v (包含%d个特定ID规则)`,
		f.Name,
		f.Private.Mode, f.Private.Ids, privateSpecificCount,
		f.Group.Mode, f.Group.Ids, groupSpecificCount)
}

// detail_type过滤
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
	}
	return true
}

// 新增：使用特定规则进行前缀通过检查
func (f *MessageTypeFilter) prefixPassWithRule(onebotMessage *OneBotMessage, specificRule *SpecificRuleFilter) bool {
	// 使用特定规则或默认规则
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

	// 查找匹配的前缀
	prefix := ""
	for _, p := range prefixes {
		if p == "" {
			continue
		}
		if strings.HasPrefix(textOld, p) {
			prefix = p
			break
		}
	}

	if prefix == "" {
		return false
	}

	// 修改匹配到前缀的消息段
	text := prefixReplace + textOld[len(prefix):]
	switch onebotMessage.Partial.MessageFormat {
	case MESSAGE_FORMAT_ARRAY:
		onebotMessage.Partial.MessageArray[index].Data["text"] = text
		if strings.TrimSpace(text) == "" {
			onebotMessage.Partial.MessageArray = append(onebotMessage.Partial.MessageArray[:index], onebotMessage.Partial.MessageArray[index+1:]...)
		}
		onebotMessage.Intact["message"], err = json.Marshal(onebotMessage.Partial.MessageArray)
		if err != nil {
			log.Println("将修改后的消息转为json字符串出错", err)
			return false
		}
	case MESSAGE_FORMAT_STRING:
		onebotMessage.Partial.MessageString = text
		onebotMessage.Intact["message"], err = json.Marshal(onebotMessage.Partial.MessageString)
		if err != nil {
			log.Println("将修改后的消息转为json字符串出错", err)
			return false
		}
	}

	// 修改原始消息
	onebotMessage.Partial.RawMessage = strings.Replace(onebotMessage.Partial.RawMessage, textOld, text, 1)
	onebotMessage.Intact["raw_message"], err = json.Marshal(onebotMessage.Partial.RawMessage)
	if err != nil {
		log.Println("将修改后的消息转为json字符串出错", err)
		return false
	}

	return true
}

// 新增：使用特定规则进行正则匹配
func (f *MessageTypeFilter) processFilterWithRule(filterName string, onebotMessage *OneBotMessage, specificRule *SpecificRuleFilter) *bool {
	ruleMode := f.getRuleMode(specificRule)
	var regexps []*regexp.Regexp

	// 使用特定规则或默认规则的正则表达式
	if specificRule != nil && len(specificRule.Regexps) > 0 {
		regexps = specificRule.Regexps
	} else {
		regexps = f.Regexps
	}

	// 处理消息文本
	if onebotMessage.Partial.MessageFormat == "array" {
		for _, message := range onebotMessage.Partial.MessageArray {
			if message.Type == MESSAGE_TYPE_TEXT {
				text := strings.TrimSpace(message.Data["text"].(string))
				for _, pattern := range regexps {
					if ok, err := pattern.MatchString(text); ok {
						switch ruleMode {
						case WHITELIST:
							log.Printf("%s：白名单的消息：%s\n", filterName, onebotMessage.Partial.RawMessage)
							return &TRUE
						case BLACKLIST:
							log.Printf("%s：黑名单的消息：%s\n", filterName, onebotMessage.Partial.RawMessage)
							return &FALSE
						}
					} else if err != nil {
						log.Printf("过滤器%s正则匹配出错的消息：%s\n", pattern.String(), onebotMessage.Partial.RawMessage)
					}
				}
			}
		}
	} else {
		text := strings.TrimSpace(onebotMessage.Partial.MessageString)
		for _, pattern := range regexps {
			if ok, err := pattern.MatchString(text); ok {
				switch ruleMode {
				case WHITELIST:
					log.Printf("%s：白名单的消息：%s\n", filterName, onebotMessage.Partial.RawMessage)
					return &TRUE
				case BLACKLIST:
					log.Printf("%s：黑名单的消息：%s\n", filterName, onebotMessage.Partial.RawMessage)
					return &FALSE
				}
			} else if err != nil {
				log.Printf("过滤器%s正则匹配出错的消息：%s\n", pattern.String(), onebotMessage.Partial.RawMessage)
			}
		}
	}

	return nil
}

// 新增：获取特定ID的规则
func (f *MessageTypeFilter) getSpecificRule(id int64) *SpecificRuleFilter {
	if f.SpecificRules == nil {
		return nil
	}
	return f.SpecificRules[id]
}

// 新增：获取规则模式
func (f *MessageTypeFilter) getRuleMode(specificRule *SpecificRuleFilter) string {
	if specificRule != nil && specificRule.Mode != "" {
		return specificRule.Mode
	}
	return f.Message.Mode
}

// 新增：获取前缀
func (f *MessageTypeFilter) getPrefix(specificRule *SpecificRuleFilter) []string {
	if specificRule != nil && len(specificRule.Prefix) > 0 {
		return specificRule.Prefix
	}
	return f.Message.Prefix
}

// 新增：获取前缀替换
func (f *MessageTypeFilter) getPrefixReplace(specificRule *SpecificRuleFilter) string {
	if specificRule != nil && specificRule.PrefixReplace != "" {
		return specificRule.PrefixReplace
	}
	return f.Message.PrefixReplace
}

// AddToBlacklist 将ID添加到黑名单并保存到配置文件
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

	// 确保使用blacklist模式
	if targetFilter.Message.Mode != BLACKLIST {
		targetFilter.Message.Mode = BLACKLIST
		log.Printf("%s：已切换%s消息模式为blacklist\n", f.Name, messageType)
	}

	// 检查是否已在黑名单中
	if slices.Contains(targetFilter.Ids, id) {
		log.Printf("%s：%s ID %d 已在黑名单中\n", f.Name, messageType, id)
		return true
	}

	// 添加到黑名单
	targetFilter.Ids = append(targetFilter.Ids, id)
	log.Printf("%s：已将%s ID %d 添加到黑名单，当前黑名单：%v\n",
		f.Name, messageType, id, targetFilter.Ids)

	// 保存到配置文件
	return f.saveBlacklistToConfig(messageType)
}

// RemoveFromBlacklist 从黑名单中移除ID并保存到配置文件
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

	// 确保使用blacklist模式
	if targetFilter.Message.Mode != BLACKLIST {
		targetFilter.Message.Mode = BLACKLIST
		log.Printf("%s：已切换%s消息模式为blacklist\n", f.Name, messageType)
	}

	// 查找并移除
	found := false
	newIds := make([]int64, 0, len(targetFilter.Ids))
	for _, existingId := range targetFilter.Ids {
		if existingId == id {
			found = true
			continue
		}
		newIds = append(newIds, existingId)
	}

	if !found {
		log.Printf("%s：%s ID %d 不在黑名单中\n", f.Name, messageType, id)
		return false
	}

	targetFilter.Ids = newIds
	log.Printf("%s：已将%s ID %d 从黑名单中移除，当前黑名单：%v\n",
		f.Name, messageType, id, targetFilter.Ids)

	// 保存到配置文件
	return f.saveBlacklistToConfig(messageType)
}

// saveBlacklistToConfig 将黑名单保存到配置文件 - 添加详细日志
func (f *Filter) saveBlacklistToConfig(messageType string) bool {
	log.Printf("%s：开始保存黑名单配置到文件，消息类型：%s，当前黑名单：%v\n",
		f.Name, messageType,
		f.GetBlacklist(messageType))

	success := UpdateBotAppConfig(f.Name, func(botApp *BotAppsConfig) {
		switch messageType {
		case PRIVATE:
			log.Printf("%s：更新配置中的私聊黑名单，从 %v 更新为 %v\n",
				f.Name, botApp.Private.Ids, f.Private.Ids)
			botApp.Private.Ids = f.Private.Ids
			// 确保模式为blacklist
			if botApp.Private.Mode != BLACKLIST {
				botApp.Private.Mode = BLACKLIST
				log.Printf("%s：已将私聊模式切换为blacklist\n", f.Name)
			}
		case GROUP:
			log.Printf("%s：更新配置中的群聊黑名单，从 %v 更新为 %v\n",
				f.Name, botApp.Group.Ids, f.Group.Ids)
			botApp.Group.Ids = f.Group.Ids
			// 确保模式为blacklist
			if botApp.Group.Mode != BLACKLIST {
				botApp.Group.Mode = BLACKLIST
				log.Printf("%s：已将群聊模式切换为blacklist\n", f.Name)
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

// GetBlacklist 获取黑名单列表
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
