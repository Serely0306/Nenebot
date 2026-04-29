package filter

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"onebotfilter/src/core"
)

const (
	commandKindBotSwitch     = "bot-switch"
	commandKindPresetSet     = "preset-set"
	commandKindPresetClear   = "preset-clear"
	commandKindGlobalBlock   = "global-block"
	commandKindGlobalUnblock = "global-unblock"
)

type controlCommand struct {
	Kind          string
	MessageType   string
	BotName       string
	TargetID      int64
	TargetLabel   string
	PresetName    string
	ReplyText     string
	RequiresAuth  bool
	SuperUserOnly bool
}

func (m *Module) TryHandle(onebotMessage *core.OneBotMessage) (bool, map[string]interface{}) {
	cmd, ok := parseControlCommand(onebotMessage)
	if !ok {
		return false, nil
	}

	if cmd.RequiresAuth && !canExecuteControlCommand(onebotMessage, cmd) {
		if core.CONFIG.Server.Debug {
			log.Printf("命令因权限不足被忽略：group=%d user=%d role=%s raw=%s\n",
				onebotMessage.Partial.GroupID,
				onebotMessage.Partial.Sender.UserID,
				onebotMessage.Partial.Sender.Role,
				onebotMessage.Partial.RawMessage,
			)
		}
		return true, nil
	}

	switch cmd.Kind {
	case commandKindBotSwitch:
		return true, buildReply(onebotMessage, executeBotSwitchCommand(cmd))
	case commandKindPresetSet:
		return true, buildReply(onebotMessage, executePresetCommand(cmd, true))
	case commandKindPresetClear:
		return true, buildReply(onebotMessage, executePresetCommand(cmd, false))
	case commandKindGlobalBlock:
		return true, buildReply(onebotMessage, executeGlobalBlockCommand(cmd, true))
	case commandKindGlobalUnblock:
		return true, buildReply(onebotMessage, executeGlobalBlockCommand(cmd, false))
	default:
		return false, nil
	}
}

func extractMessageText(onebotMessage *core.OneBotMessage) string {
	switch onebotMessage.Partial.MessageFormat {
	case MESSAGE_FORMAT_ARRAY:
		for _, msg := range onebotMessage.Partial.MessageArray {
			if msg.Type != MESSAGE_TYPE_TEXT {
				continue
			}
			text, _ := msg.Data["text"].(string)
			return strings.TrimSpace(text)
		}
	case MESSAGE_FORMAT_STRING:
		return strings.TrimSpace(onebotMessage.Partial.MessageString)
	}
	return ""
}

func parseControlCommand(onebotMessage *core.OneBotMessage) (*controlCommand, bool) {
	if onebotMessage == nil {
		return nil, false
	}

	messageText := extractMessageText(onebotMessage)
	if messageText == "" {
		return nil, false
	}

	parts := strings.Fields(messageText)
	if len(parts) == 0 {
		return nil, false
	}

	if onebotMessage.Partial.MessageType == GROUP && onebotMessage.Partial.GroupID > 0 {
		if cmd, ok := parseBotSwitchCommand(parts, onebotMessage.Partial.GroupID); ok {
			return cmd, true
		}
	}
	if cmd, ok := parsePresetCommand(parts, onebotMessage); ok {
		return cmd, true
	}
	if cmd, ok := parseGlobalBlockCommand(parts, onebotMessage); ok {
		return cmd, true
	}

	return nil, false
}

func parseBotSwitchCommand(parts []string, currentGroupID int64) (*controlCommand, bool) {
	if len(parts) != 2 {
		return nil, false
	}

	switch parts[0] {
	case "/启用":
		return &controlCommand{Kind: commandKindBotSwitch, MessageType: GROUP, BotName: parts[1], TargetID: currentGroupID, TargetLabel: "群聊", ReplyText: "启用", RequiresAuth: true}, true
	case "/禁用":
		return &controlCommand{Kind: commandKindBotSwitch, MessageType: GROUP, BotName: parts[1], TargetID: currentGroupID, TargetLabel: "群聊", ReplyText: "禁用", RequiresAuth: true}, true
	default:
		return nil, false
	}
}

func parsePresetCommand(parts []string, onebotMessage *core.OneBotMessage) (*controlCommand, bool) {
	if len(parts) == 0 {
		return nil, false
	}

	switch parts[0] {
	case "/preset", "/setpreset", "/设置预设":
		if len(parts) != 3 && len(parts) != 4 {
			return nil, false
		}

		var targetID int64
		presetName := ""
		switch len(parts) {
		case 3:
			if onebotMessage == nil || onebotMessage.Partial.MessageType != GROUP || onebotMessage.Partial.GroupID <= 0 {
				return nil, false
			}
			targetID = onebotMessage.Partial.GroupID
			presetName = parts[2]
		case 4:
			var ok bool
			targetID, ok = parseNumericID(parts[2])
			if !ok {
				return nil, false
			}
			presetName = parts[3]
		}

		presetName = strings.TrimSpace(presetName)
		if presetName == "" {
			return nil, false
		}

		return &controlCommand{
			Kind:          commandKindPresetSet,
			MessageType:   GROUP,
			BotName:       parts[1],
			TargetID:      targetID,
			TargetLabel:   "群聊",
			PresetName:    presetName,
			RequiresAuth:  true,
			SuperUserOnly: true,
		}, true

	case "/clearpreset", "/清除预设":
		if len(parts) != 2 && len(parts) != 3 {
			return nil, false
		}

		var targetID int64
		switch len(parts) {
		case 2:
			if onebotMessage == nil || onebotMessage.Partial.MessageType != GROUP || onebotMessage.Partial.GroupID <= 0 {
				return nil, false
			}
			targetID = onebotMessage.Partial.GroupID
		case 3:
			var ok bool
			targetID, ok = parseNumericID(parts[2])
			if !ok {
				return nil, false
			}
		}

		return &controlCommand{
			Kind:          commandKindPresetClear,
			MessageType:   GROUP,
			BotName:       parts[1],
			TargetID:      targetID,
			TargetLabel:   "群聊",
			RequiresAuth:  true,
			SuperUserOnly: true,
		}, true
	}

	return nil, false
}

func parseGlobalBlockCommand(parts []string, onebotMessage *core.OneBotMessage) (*controlCommand, bool) {
	switch parts[0] {
	case "/拉黑群聊", "/静默群聊":
		targetID, ok := parseOptionalGroupID(parts, onebotMessage)
		if !ok {
			return nil, false
		}
		replyText := "拉黑"
		if parts[0] == "/静默群聊" {
			replyText = "静默"
		}
		return &controlCommand{Kind: commandKindGlobalBlock, MessageType: GROUP, TargetID: targetID, TargetLabel: "群聊", ReplyText: replyText, RequiresAuth: true, SuperUserOnly: true}, true

	case "/取消拉黑群聊", "/取消静默群聊":
		targetID, ok := parseOptionalGroupID(parts, onebotMessage)
		if !ok {
			return nil, false
		}
		replyText := "取消拉黑"
		if parts[0] == "/取消静默群聊" {
			replyText = "取消静默"
		}
		return &controlCommand{Kind: commandKindGlobalUnblock, MessageType: GROUP, TargetID: targetID, TargetLabel: "群聊", ReplyText: replyText, RequiresAuth: true, SuperUserOnly: true}, true

	case "/拉黑用户", "/静默用户":
		if len(parts) != 2 {
			return nil, false
		}
		targetID, ok := parseNumericID(parts[1])
		if !ok {
			return nil, false
		}
		replyText := "拉黑"
		if parts[0] == "/静默用户" {
			replyText = "静默"
		}
		return &controlCommand{Kind: commandKindGlobalBlock, MessageType: PRIVATE, TargetID: targetID, TargetLabel: "用户", ReplyText: replyText, RequiresAuth: true, SuperUserOnly: true}, true

	case "/取消拉黑用户", "/取消静默用户":
		if len(parts) != 2 {
			return nil, false
		}
		targetID, ok := parseNumericID(parts[1])
		if !ok {
			return nil, false
		}
		replyText := "取消拉黑"
		if parts[0] == "/取消静默用户" {
			replyText = "取消静默"
		}
		return &controlCommand{Kind: commandKindGlobalUnblock, MessageType: PRIVATE, TargetID: targetID, TargetLabel: "用户", ReplyText: replyText, RequiresAuth: true, SuperUserOnly: true}, true
	}

	return nil, false
}

func parseOptionalGroupID(parts []string, onebotMessage *core.OneBotMessage) (int64, bool) {
	switch len(parts) {
	case 1:
		if onebotMessage == nil || onebotMessage.Partial.MessageType != GROUP {
			return 0, false
		}
		return onebotMessage.Partial.GroupID, onebotMessage.Partial.GroupID > 0
	case 2:
		return parseNumericID(parts[1])
	default:
		return 0, false
	}
}

func parseNumericID(raw string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func canExecuteControlCommand(onebotMessage *core.OneBotMessage, cmd *controlCommand) bool {
	auth := core.CONFIG.Server.CommandAuth
	userID := getMessageUserID(onebotMessage)

	if cmd != nil && cmd.SuperUserOnly {
		return auth.IsSuperUser(userID)
	}
	if !auth.Enabled {
		return true
	}
	if auth.IsSuperUser(userID) {
		return true
	}

	switch onebotMessage.Partial.Sender.Role {
	case "owner":
		return auth.AllowOwner
	case "admin":
		return auth.AllowAdmin
	default:
		return false
	}
}

func buildReply(onebotMessage *core.OneBotMessage, message interface{}) map[string]interface{} {
	switch onebotMessage.Partial.MessageType {
	case PRIVATE:
		return map[string]interface{}{
			"action": "send_private_msg",
			"params": map[string]interface{}{
				"user_id": onebotMessage.Partial.UserID,
				"message": message,
			},
		}
	default:
		return map[string]interface{}{
			"action": "send_group_msg",
			"params": map[string]interface{}{
				"group_id": onebotMessage.Partial.GroupID,
				"message":  message,
			},
		}
	}
}

func buildForwardReply(onebotMessage *core.OneBotMessage, messages []map[string]interface{}) map[string]interface{} {
	switch onebotMessage.Partial.MessageType {
	case PRIVATE:
		return map[string]interface{}{
			"action": "send_private_forward_msg",
			"params": map[string]interface{}{
				"user_id":  onebotMessage.Partial.UserID,
				"messages": messages,
			},
		}
	default:
		return map[string]interface{}{
			"action": "send_group_forward_msg",
			"params": map[string]interface{}{
				"group_id": onebotMessage.Partial.GroupID,
				"messages": messages,
			},
		}
	}
}

func executeBotSwitchCommand(cmd *controlCommand) string {
	filter := getCompiledFilter(cmd.BotName)
	if filter == nil {
		return fmt.Sprintf("未找到 bot：%s", cmd.BotName)
	}

	switch cmd.ReplyText {
	case "禁用":
		if filter.AddToBlacklist(cmd.MessageType, cmd.TargetID) {
			return fmt.Sprintf("%s禁用成功", cmd.BotName)
		}
		return fmt.Sprintf("%s禁用失败", cmd.BotName)
	case "启用":
		if filter.RemoveFromBlacklist(cmd.MessageType, cmd.TargetID) {
			return fmt.Sprintf("%s启用成功", cmd.BotName)
		}
		return fmt.Sprintf("%s启用失败", cmd.BotName)
	default:
		return fmt.Sprintf("未找到 bot：%s", cmd.BotName)
	}
}

func executeGlobalBlockCommand(cmd *controlCommand, shouldBlock bool) string {
	ok := updateServerConfig(func(server *core.ServerConfig) {
		switch cmd.MessageType {
		case PRIVATE:
			if shouldBlock {
				server.Blocked.UserIDs = appendUniqueID(server.Blocked.UserIDs, cmd.TargetID)
			} else {
				server.Blocked.UserIDs = removeID(server.Blocked.UserIDs, cmd.TargetID)
			}
		case GROUP:
			if shouldBlock {
				server.Blocked.GroupIDs = appendUniqueID(server.Blocked.GroupIDs, cmd.TargetID)
			} else {
				server.Blocked.GroupIDs = removeID(server.Blocked.GroupIDs, cmd.TargetID)
			}
		}
	})
	if !ok {
		return fmt.Sprintf("%s%s失败：%d", cmd.ReplyText, cmd.TargetLabel, cmd.TargetID)
	}
	return fmt.Sprintf("%s%s成功：%d", cmd.ReplyText, cmd.TargetLabel, cmd.TargetID)
}

func executePresetCommand(cmd *controlCommand, shouldSet bool) string {
	ok := updateBotAppConfig(cmd.BotName, func(botApp *core.BotAppsConfig) {
		target := &botApp.Group
		if target.Presets == nil {
			target.Presets = make(map[string]string)
		}
		key := strconv.FormatInt(cmd.TargetID, 10)
		if shouldSet {
			if strings.TrimSpace(target.Preset) == strings.TrimSpace(cmd.PresetName) {
				delete(target.Presets, key)
			} else {
				target.Presets[key] = cmd.PresetName
			}
			return
		}
		delete(target.Presets, key)
	})
	if !ok {
		if shouldSet {
			return fmt.Sprintf("%s 预设切换失败: %d -> %s", cmd.BotName, cmd.TargetID, cmd.PresetName)
		}
		return fmt.Sprintf("%s 预设清除失败: %d", cmd.BotName, cmd.TargetID)
	}
	if shouldSet {
		return fmt.Sprintf("%s 已为群聊 %d 切换到预设 %s", cmd.BotName, cmd.TargetID, cmd.PresetName)
	}
	return fmt.Sprintf("%s 已清除群聊 %d 的单独预设", cmd.BotName, cmd.TargetID)
}

func appendUniqueID(values []int64, target int64) []int64 {
	if slicesContains(values, target) {
		return values
	}
	return append(values, target)
}

func removeID(values []int64, target int64) []int64 {
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value == target {
			continue
		}
		result = append(result, value)
	}
	return result
}

func slicesContains(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func getMessageUserID(onebotMessage *core.OneBotMessage) int64 {
	if onebotMessage == nil {
		return 0
	}
	if onebotMessage.Partial.Sender.UserID > 0 {
		return onebotMessage.Partial.Sender.UserID
	}
	return onebotMessage.Partial.UserID
}
