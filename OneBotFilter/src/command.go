package onebotfilter

import (
	"fmt"
	"log"
	"strconv"
	"strings"
)

const (
	commandKindHelp          = "help"
	commandKindBotSwitch     = "bot-switch"
	commandKindGlobalBlock   = "global-block"
	commandKindGlobalUnblock = "global-unblock"
)

type controlCommand struct {
	Kind          string
	MessageType   string
	BotName       string
	TargetID      int64
	TargetLabel   string
	ReplyText     string
	RequiresAuth  bool
	SuperUserOnly bool
}

func extractMessageText(onebotMessage *OneBotMessage) string {
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

func parseControlCommand(onebotMessage *OneBotMessage) (*controlCommand, bool) {
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

	if cmd, ok := parseHelpCommand(parts); ok {
		return cmd, true
	}

	if onebotMessage.Partial.MessageType == GROUP && onebotMessage.Partial.GroupId > 0 {
		if cmd, ok := parseBotSwitchCommand(parts, onebotMessage.Partial.GroupId); ok {
			return cmd, true
		}
	}

	if cmd, ok := parseGlobalBlockCommand(parts, onebotMessage); ok {
		return cmd, true
	}

	return nil, false
}

func parseHelpCommand(parts []string) (*controlCommand, bool) {
	if len(parts) != 1 {
		return nil, false
	}

	switch parts[0] {
	case "help", "/help", "帮助", "/帮助":
		return &controlCommand{
			Kind:         commandKindHelp,
			RequiresAuth: false,
		}, true
	default:
		return nil, false
	}
}

func parseBotSwitchCommand(parts []string, currentGroupID int64) (*controlCommand, bool) {
	if len(parts) != 2 {
		return nil, false
	}

	switch parts[0] {
	case "/启用":
		return &controlCommand{
			Kind:         commandKindBotSwitch,
			MessageType:  GROUP,
			BotName:      parts[1],
			TargetID:     currentGroupID,
			TargetLabel:  "群聊",
			ReplyText:    "启用",
			RequiresAuth: true,
		}, true
	case "/禁用":
		return &controlCommand{
			Kind:         commandKindBotSwitch,
			MessageType:  GROUP,
			BotName:      parts[1],
			TargetID:     currentGroupID,
			TargetLabel:  "群聊",
			ReplyText:    "禁用",
			RequiresAuth: true,
		}, true
	default:
		return nil, false
	}
}

func parseGlobalBlockCommand(parts []string, onebotMessage *OneBotMessage) (*controlCommand, bool) {
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
		return &controlCommand{
			Kind:          commandKindGlobalBlock,
			MessageType:   GROUP,
			TargetID:      targetID,
			TargetLabel:   "群聊",
			ReplyText:     replyText,
			RequiresAuth:  true,
			SuperUserOnly: true,
		}, true
	case "/取消拉黑群聊", "/取消静默群聊":
		targetID, ok := parseOptionalGroupID(parts, onebotMessage)
		if !ok {
			return nil, false
		}
		replyText := "取消拉黑"
		if parts[0] == "/取消静默群聊" {
			replyText = "取消静默"
		}
		return &controlCommand{
			Kind:          commandKindGlobalUnblock,
			MessageType:   GROUP,
			TargetID:      targetID,
			TargetLabel:   "群聊",
			ReplyText:     replyText,
			RequiresAuth:  true,
			SuperUserOnly: true,
		}, true
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
		return &controlCommand{
			Kind:          commandKindGlobalBlock,
			MessageType:   PRIVATE,
			TargetID:      targetID,
			TargetLabel:   "用户",
			ReplyText:     replyText,
			RequiresAuth:  true,
			SuperUserOnly: true,
		}, true
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
		return &controlCommand{
			Kind:          commandKindGlobalUnblock,
			MessageType:   PRIVATE,
			TargetID:      targetID,
			TargetLabel:   "用户",
			ReplyText:     replyText,
			RequiresAuth:  true,
			SuperUserOnly: true,
		}, true
	default:
		return nil, false
	}
}

func parseOptionalGroupID(parts []string, onebotMessage *OneBotMessage) (int64, bool) {
	switch len(parts) {
	case 1:
		if onebotMessage == nil || onebotMessage.Partial.MessageType != GROUP {
			return 0, false
		}
		return onebotMessage.Partial.GroupId, onebotMessage.Partial.GroupId > 0
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

func (cfg CommandAuthConfig) isSuperUser(userID int64) bool {
	return slicesContains(cfg.SuperUsers, userID)
}

func canExecuteControlCommand(onebotMessage *OneBotMessage, cmd *controlCommand) bool {
	auth := CONFIG.Server.CommandAuth

	userID := getMessageUserID(onebotMessage)

	if cmd != nil && cmd.SuperUserOnly {
		return auth.isSuperUser(userID)
	}

	if !auth.Enabled {
		return true
	}

	if auth.isSuperUser(userID) {
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

func buildReply(onebotMessage *OneBotMessage, message interface{}) map[string]interface{} {
	switch onebotMessage.Partial.MessageType {
	case PRIVATE:
		return map[string]interface{}{
			"action": "send_private_msg",
			"params": map[string]interface{}{
				"user_id": onebotMessage.Partial.UserId,
				"message": message,
			},
		}
	default:
		return map[string]interface{}{
			"action": "send_group_msg",
			"params": map[string]interface{}{
				"group_id": onebotMessage.Partial.GroupId,
				"message":  message,
			},
		}
	}
}

func buildForwardReply(onebotMessage *OneBotMessage, messages []map[string]interface{}) map[string]interface{} {
	switch onebotMessage.Partial.MessageType {
	case PRIVATE:
		return map[string]interface{}{
			"action": "send_private_forward_msg",
			"params": map[string]interface{}{
				"user_id":  onebotMessage.Partial.UserId,
				"messages": messages,
			},
		}
	default:
		return map[string]interface{}{
			"action": "send_group_forward_msg",
			"params": map[string]interface{}{
				"group_id": onebotMessage.Partial.GroupId,
				"messages": messages,
			},
		}
	}
}

func handleControlCommand(onebotMessage *OneBotMessage) (bool, map[string]interface{}) {
	cmd, ok := parseControlCommand(onebotMessage)
	if !ok {
		return false, nil
	}

	if cmd.Kind == commandKindHelp {
		helpMessage, enabled, err := buildUnifiedHelpMessage()
		if err != nil {
			return true, buildReply(onebotMessage, fmt.Sprintf("\u7edf\u4e00\u5e2e\u52a9\u751f\u6210\u5931\u8d25: %v", err))
		}
		if !enabled {
			return false, nil
		}
		if isGloballyBlocked(onebotMessage) {
			return true, nil
		}
		return true, buildForwardReply(onebotMessage, helpMessage)
	}

	if cmd.RequiresAuth && !canExecuteControlCommand(onebotMessage, cmd) {
		if CONFIG.Server.Debug {
			log.Printf("命令因权限不足被忽略：group=%d user=%d role=%s raw=%s\n",
				onebotMessage.Partial.GroupId,
				onebotMessage.Partial.Sender.UserId,
				onebotMessage.Partial.Sender.Role,
				onebotMessage.Partial.RawMessage,
			)
		}
		return true, nil
	}

	switch cmd.Kind {
	case commandKindBotSwitch:
		return true, buildReply(onebotMessage, executeBotSwitchCommand(cmd))
	case commandKindGlobalBlock:
		return true, buildReply(onebotMessage, executeGlobalBlockCommand(cmd, true))
	case commandKindGlobalUnblock:
		return true, buildReply(onebotMessage, executeGlobalBlockCommand(cmd, false))
	default:
		return false, nil
	}
}

func executeBotSwitchCommand(cmd *controlCommand) string {
	for _, filter := range ALL_FILTERS {
		if filter.Name != cmd.BotName {
			continue
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
		}
	}

	return fmt.Sprintf("未找到 bot：%s", cmd.BotName)
}

func executeGlobalBlockCommand(cmd *controlCommand, shouldBlock bool) string {
	ok := UpdateServerConfig(func(server *ServerConfig) {
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
