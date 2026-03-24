package onebotfilter

import (
	"encoding/json"
	"log"
)

type OneBotMessage struct {
	Raw     []byte
	Partial OneBotMessagePartial
	Intact  map[string]json.RawMessage
}

type OneBotMessagePartial struct {
	MessageType      string           `json:"message_type"`
	MessageFormat    string           `json:"message_format"`
	UnDecodedMessage json.RawMessage  `json:"message"`
	MessageArray     []MessageContent `json:"-"`
	MessageString    string           `json:"-"`
	UserId           int64            `json:"user_id"`
	GroupId          int64            `json:"group_id"`
	RawMessage       string           `json:"raw_message"`
	Sender           OneBotSender     `json:"sender"`
}

type OneBotSender struct {
	UserId int64  `json:"user_id"`
	Role   string `json:"role"`
}

type MessageContent struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

func ParseOneBotMessage(raw []byte) *OneBotMessage {
	oneBotMessage := &OneBotMessage{Raw: raw}

	if err := json.Unmarshal(raw, &oneBotMessage.Intact); err != nil {
		return nil
	}
	if err := json.Unmarshal(raw, &oneBotMessage.Partial); err != nil {
		return nil
	}

	switch oneBotMessage.Partial.MessageFormat {
	case MESSAGE_FORMAT_ARRAY:
		if err := json.Unmarshal(oneBotMessage.Partial.UnDecodedMessage, &oneBotMessage.Partial.MessageArray); err != nil {
			log.Printf("将消息解析为数组失败：%s\n", oneBotMessage.Partial.UnDecodedMessage)
			return nil
		}
	case MESSAGE_FORMAT_STRING:
		if err := json.Unmarshal(oneBotMessage.Partial.UnDecodedMessage, &oneBotMessage.Partial.MessageString); err != nil {
			log.Printf("将消息解析为字符串失败：%s\n", oneBotMessage.Partial.UnDecodedMessage)
			return nil
		}
	default:
		return nil
	}

	return oneBotMessage
}

func getMessageUserID(onebotMessage *OneBotMessage) int64 {
	if onebotMessage == nil {
		return 0
	}

	if onebotMessage.Partial.Sender.UserId > 0 {
		return onebotMessage.Partial.Sender.UserId
	}

	return onebotMessage.Partial.UserId
}
