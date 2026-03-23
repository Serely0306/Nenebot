package onebotfilter

import (
	"fmt"
	"log"
	"strconv"
	"strings"
)

type controlCommand struct {
	Action        string
	BotName       string
	MessageType   string
	TargetID      int64
	SourceGroupID int64
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
	if onebotMessage == nil || onebotMessage.Partial.MessageType != GROUP {
		return nil, false
	}

	messageText := extractMessageText(onebotMessage)
	if messageText == "" {
		return nil, false
	}

	parts := strings.Fields(messageText)
	if len(parts) < 2 || len(parts) > 3 {
		return nil, false
	}

	cmd := &controlCommand{
		Action:        parts[0],
		BotName:       parts[1],
		SourceGroupID: onebotMessage.Partial.GroupId,
	}

	switch cmd.Action {
	case "/启用":
		if len(parts) != 2 || onebotMessage.Partial.GroupId == 0 {
			return nil, false
		}
		cmd.MessageType = GROUP
		cmd.TargetID = onebotMessage.Partial.GroupId
		return cmd, true
	case "/禁用":
		if len(parts) != 2 || onebotMessage.Partial.GroupId == 0 {
			return nil, false
		}
		cmd.MessageType = GROUP
		cmd.TargetID = onebotMessage.Partial.GroupId
		return cmd, true
	case "/拉黑群聊", "/静默群聊", "/取消拉黑群聊", "/取消静默群聊":
		cmd.MessageType = GROUP
		targetID, ok := parseGroupTargetID(parts, onebotMessage.Partial.GroupId)
		if !ok {
			return nil, false
		}
		cmd.TargetID = targetID
		return cmd, true
	case "/拉黑用户", "/静默用户", "/取消拉黑用户", "/取消静默用户":
		if len(parts) != 3 {
			return nil, false
		}
		targetID, ok := parseNumericID(parts[2])
		if !ok {
			return nil, false
		}
		cmd.MessageType = PRIVATE
		cmd.TargetID = targetID
		return cmd, true
	default:
		return nil, false
	}
}

func parseGroupTargetID(parts []string, defaultGroupID int64) (int64, bool) {
	if len(parts) == 2 {
		return defaultGroupID, defaultGroupID > 0
	}
	return parseNumericID(parts[2])
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

func canExecuteControlCommand(onebotMessage *OneBotMessage) bool {
	auth := CONFIG.Server.CommandAuth
	if !auth.Enabled {
		return true
	}

	userID := onebotMessage.Partial.Sender.UserId
	if userID == 0 {
		userID = onebotMessage.Partial.UserId
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

func buildGroupReply(groupID int64, message string) map[string]interface{} {
	return map[string]interface{}{
		"action": "send_group_msg",
		"params": map[string]interface{}{
			"group_id": groupID,
			"message":  message,
		},
	}
}

func handleControlCommand(onebotMessage *OneBotMessage) (bool, map[string]interface{}) {
	cmd, ok := parseControlCommand(onebotMessage)
	if !ok {
		return false, nil
	}

	if !canExecuteControlCommand(onebotMessage) {
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

	for _, filter := range ALL_FILTERS {
		if filter.Name != cmd.BotName {
			continue
		}

		responseMsg := executeControlCommand(filter, cmd)
		return true, buildGroupReply(cmd.SourceGroupID, responseMsg)
	}

	return true, buildGroupReply(cmd.SourceGroupID, fmt.Sprintf("未找到 bot：%s", cmd.BotName))
}

func executeControlCommand(filter *Filter, cmd *controlCommand) string {
	switch cmd.Action {
	case "/禁用":
		if filter.AddToBlacklist(GROUP, cmd.TargetID) {
			return fmt.Sprintf("%s禁用成功", cmd.BotName)
		}
		return fmt.Sprintf("%s禁用失败", cmd.BotName)
	case "/启用":
		if filter.RemoveFromBlacklist(GROUP, cmd.TargetID) {
			return fmt.Sprintf("%s启用成功", cmd.BotName)
		}
		return fmt.Sprintf("%s启用失败", cmd.BotName)
	case "/拉黑群聊", "/静默群聊", "/拉黑用户", "/静默用户":
		if filter.AddToBlacklist(cmd.MessageType, cmd.TargetID) {
			return fmt.Sprintf("%s%s成功：%d", cmd.BotName, strings.TrimPrefix(cmd.Action, "/"), cmd.TargetID)
		}
		return fmt.Sprintf("%s%s失败：%d", cmd.BotName, strings.TrimPrefix(cmd.Action, "/"), cmd.TargetID)
	case "/取消拉黑群聊", "/取消静默群聊", "/取消拉黑用户", "/取消静默用户":
		if filter.RemoveFromBlacklist(cmd.MessageType, cmd.TargetID) {
			return fmt.Sprintf("%s%s成功：%d", cmd.BotName, strings.TrimPrefix(cmd.Action, "/"), cmd.TargetID)
		}
		return fmt.Sprintf("%s%s失败：%d", cmd.BotName, strings.TrimPrefix(cmd.Action, "/"), cmd.TargetID)
	default:
		return "未识别的命令"
	}
}

func slicesContains(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
