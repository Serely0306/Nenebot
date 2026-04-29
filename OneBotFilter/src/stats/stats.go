package stats

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	"onebotfilter/src/core"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Enabled                 bool     `mapstructure:"enabled"`
	PrivateLabel            string   `mapstructure:"private_label"`
	DefaultRankLimit        int      `mapstructure:"default_rank_limit"`
	MaxRankLimit            int      `mapstructure:"max_rank_limit"`
	ImageWidth              int      `mapstructure:"image_width"`
	QueueSize               int      `mapstructure:"queue_size"`
	CountMessageSent        bool     `mapstructure:"count_message_sent"`
	CountFilterInternalSend bool     `mapstructure:"count_filter_internal_send"`
	SendActionWhitelist     []string `mapstructure:"send_action_whitelist"`
}

type NameResolver interface {
	ResolveGroupMemberName(groupID, userID int64) (string, error)
	ResolvePrivateName(userID int64) (string, error)
}

type Module struct {
	cfg       Config
	store     *Store
	queue     chan Event
	stop      chan struct{}
	resolver  NameResolver
	superUser func(int64) bool
}

func NewModule(cfg Config, store *Store, resolver NameResolver, superUser func(int64) bool) *Module {
	queueSize := cfg.QueueSize
	if queueSize <= 0 {
		queueSize = 256
	}
	return &Module{
		cfg:       cfg,
		store:     store,
		queue:     make(chan Event, queueSize),
		stop:      make(chan struct{}),
		resolver:  resolver,
		superUser: superUser,
	}
}

func (m *Module) Start() {
	if m == nil || m.store == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		batch := make([]Event, 0, 128)

		flush := func() {
			if len(batch) == 0 {
				return
			}
			if err := m.store.RecordBatch(batch); err != nil {
				log.Printf("stats flush failed: %v\n", err)
			}
			batch = batch[:0]
		}

		for {
			select {
			case event := <-m.queue:
				batch = append(batch, event)
				if len(batch) >= 128 {
					flush()
				}
			case <-ticker.C:
				flush()
			case <-m.stop:
				flush()
				return
			}
		}
	}()
}

func (m *Module) Stop() {
	if m == nil {
		return
	}
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
}

func (m *Module) ShouldCountIncoming(msg *core.OneBotMessage) bool {
	if m == nil || msg == nil {
		return false
	}

	switch msg.Partial.PostType {
	case "message":
		return msg.Partial.MessageType == "group" || msg.Partial.MessageType == "private"
	case "message_sent":
		return m.cfg.CountMessageSent && (msg.Partial.MessageType == "group" || msg.Partial.MessageType == "private")
	default:
		return false
	}
}

func (m *Module) ShouldCountOutgoing(action string) bool {
	if m == nil {
		return false
	}
	action = strings.TrimSpace(action)
	for _, item := range m.cfg.SendActionWhitelist {
		if action == strings.TrimSpace(item) {
			return true
		}
	}
	return false
}

func (m *Module) Enqueue(event Event) bool {
	if m == nil || !m.cfg.Enabled {
		return false
	}
	select {
	case m.queue <- event:
		return true
	default:
		return false
	}
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
	return cfg, nil
}

func (m *Module) HandleUpstreamEvent(msg *core.OneBotMessage) {
	if !m.ShouldCountIncoming(msg) {
		return
	}

	sessionType, sessionID := classifyIncomingSession(msg)
	if sessionType == "" {
		return
	}

	snapshot := strings.TrimSpace(msg.Partial.Sender.Card)
	if snapshot == "" {
		snapshot = strings.TrimSpace(msg.Partial.Sender.Nickname)
	}

	_ = m.Enqueue(Event{
		EventTime:        time.Now(),
		EventDate:        time.Now().Format("2006-01-02"),
		SessionType:      sessionType,
		SessionID:        sessionID,
		Direction:        "recv",
		SourceType:       "upstream",
		UserID:           core.GetMessageUserID(msg),
		NicknameSnapshot: snapshot,
		MessageType:      msg.Partial.MessageType,
		ActionName:       msg.Partial.PostType,
	})
}

func (m *Module) HandleBotAction(botName string, msgType int, msgData []byte) {
	if msgType != 1 {
		return
	}

	action, sessionType, sessionID, ok := parseOutgoingAction(msgData)
	if !ok || !m.ShouldCountOutgoing(action) {
		return
	}

	now := time.Now()
	_ = m.Enqueue(Event{
		EventTime:   now,
		EventDate:   now.Format("2006-01-02"),
		SessionType: sessionType,
		SessionID:   sessionID,
		Direction:   "send",
		SourceType:  "bot_app",
		BotName:     botName,
		ActionName:  action,
	})
}

func (m *Module) HandleInternalSend(response map[string]interface{}) {
	if !m.cfg.CountFilterInternalSend || response == nil {
		return
	}

	action, sessionType, sessionID, ok := parseOutgoingResponse(response)
	if !ok || !m.ShouldCountOutgoing(action) {
		return
	}

	now := time.Now()
	_ = m.Enqueue(Event{
		EventTime:   now,
		EventDate:   now.Format("2006-01-02"),
		SessionType: sessionType,
		SessionID:   sessionID,
		Direction:   "send",
		SourceType:  "filter_internal",
		ActionName:  action,
	})
}

func classifyIncomingSession(msg *core.OneBotMessage) (string, int64) {
	switch msg.Partial.MessageType {
	case "group":
		if msg.Partial.GroupID <= 0 {
			return "", 0
		}
		return "group", msg.Partial.GroupID
	case "private":
		return "private", 0
	default:
		return "", 0
	}
}

func parseOutgoingAction(msgData []byte) (string, string, int64, bool) {
	var payload map[string]interface{}
	if err := json.Unmarshal(msgData, &payload); err != nil {
		return "", "", 0, false
	}
	return parseOutgoingPayload(payload)
}

func parseOutgoingResponse(payload map[string]interface{}) (string, string, int64, bool) {
	return parseOutgoingPayload(payload)
}

func parseOutgoingPayload(payload map[string]interface{}) (string, string, int64, bool) {
	action, _ := payload["action"].(string)
	params, _ := payload["params"].(map[string]interface{})
	if action == "" || params == nil {
		return "", "", 0, false
	}

	switch action {
	case "send_group_msg", "send_group_forward_msg":
		groupID := toInt64(params["group_id"])
		if groupID <= 0 {
			return "", "", 0, false
		}
		return action, "group", groupID, true
	case "send_private_msg", "send_private_forward_msg":
		return action, "private", 0, true
	case "send_msg":
		messageType, _ := params["message_type"].(string)
		groupID := toInt64(params["group_id"])
		if messageType == "group" || groupID > 0 {
			if groupID <= 0 {
				return "", "", 0, false
			}
			return action, "group", groupID, true
		}
		return action, "private", 0, true
	default:
		return "", "", 0, false
	}
}

func toInt64(v interface{}) int64 {
	switch value := v.(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	case json.Number:
		n, _ := value.Int64()
		return n
	default:
		return 0
	}
}
