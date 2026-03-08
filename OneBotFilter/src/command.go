package onebotfilter

import (
	"fmt"
	"log"
	"strings"
)

type controlCommand struct {
	Action  string
	BotName string
	GroupId int64
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
	if len(parts) != 2 {
		return nil, false
	}
	if parts[0] != "/启用" && parts[0] != "/禁用" {
		return nil, false
	}
	if onebotMessage.Partial.GroupId == 0 {
		return nil, false
	}

	return &controlCommand{
		Action:  parts[0],
		BotName: parts[1],
		GroupId: onebotMessage.Partial.GroupId,
	}, true
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

		var success bool
		var responseMsg string
		switch cmd.Action {
		case "/禁用":
			success = filter.AddToBlacklist(GROUP, cmd.GroupId)
			if success {
				responseMsg = fmt.Sprintf("%s禁用成功", cmd.BotName)
			} else {
				responseMsg = fmt.Sprintf("%s禁用失败", cmd.BotName)
			}
		case "/启用":
			success = filter.RemoveFromBlacklist(GROUP, cmd.GroupId)
			if success {
				responseMsg = fmt.Sprintf("%s启用成功", cmd.BotName)
			} else {
				responseMsg = fmt.Sprintf("%s启用失败", cmd.BotName)
			}
		}

		return true, buildGroupReply(cmd.GroupId, responseMsg)
	}

	if CONFIG.Server.Debug {
		log.Printf("命令目标过滤器不存在，已忽略：action=%s bot=%s group=%d\n", cmd.Action, cmd.BotName, cmd.GroupId)
	}
	return true, nil
}

func slicesContains(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
