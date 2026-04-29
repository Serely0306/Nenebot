package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	MessageFormatArray  = "array"
	MessageFormatString = "string"
	MessageTypeText     = "text"
)

type WsMsg struct {
	MsgType int
	MsgData []byte
}

type MessageFilter interface {
	Filter(*OneBotMessage) bool
	String() string
}

type ExternalCommandHandler func(*OneBotMessage) (bool, map[string]interface{})
type ExternalCommandMatcher func(*OneBotMessage) bool
type ExternalCommandExecutor func(*OneBotMessage) map[string]interface{}
type UpstreamEventHook func(*OneBotMessage)
type BotActionHook func(string, int, []byte)
type InternalSendHook func(map[string]interface{})

type ExternalCommandRoute struct {
	Name    string
	Match   ExternalCommandMatcher
	Execute ExternalCommandExecutor
}

type externalCommandTask struct {
	route ExternalCommandRoute
	msg   *OneBotMessage
}

type WsServer struct {
	Conn         *websocket.Conn
	WsClients    []*WsClient
	BotID        string
	readChan     chan WsMsg
	writeChan    chan WsMsg
	commandQueue chan externalCommandTask
	apiWaiters   map[string]chan map[string]interface{}
	apiWaitersMu sync.Mutex
	echoSeq      uint64

	externalCommandHandlers []ExternalCommandHandler
	externalCommandRoutes   []ExternalCommandRoute
	upstreamEventHooks      []UpstreamEventHook
	botActionHooks          []BotActionHook
	internalSendHooks       []InternalSendHook
}

type WsClient struct {
	Name      string
	URI       string
	Access    string
	conn      *websocket.Conn
	filter    MessageFilter
	readChan  chan WsMsg
	writeChan chan WsMsg
}

func (wss *WsServer) SetExternalCommandHandlers(handlers ...ExternalCommandHandler) {
	wss.externalCommandHandlers = append([]ExternalCommandHandler(nil), handlers...)
}

func (wss *WsServer) SetExternalCommandRoutes(routes ...ExternalCommandRoute) {
	wss.externalCommandRoutes = append([]ExternalCommandRoute(nil), routes...)
}

func (wss *WsServer) SetUpstreamEventHooks(hooks ...UpstreamEventHook) {
	wss.upstreamEventHooks = append([]UpstreamEventHook(nil), hooks...)
}

func (wss *WsServer) SetBotActionHooks(hooks ...BotActionHook) {
	wss.botActionHooks = append([]BotActionHook(nil), hooks...)
}

func (wss *WsServer) SetInternalSendHooks(hooks ...InternalSendHook) {
	wss.internalSendHooks = append([]InternalSendHook(nil), hooks...)
}

type OneBotMessage struct {
	Raw     []byte
	Partial OneBotMessagePartial
	Intact  map[string]json.RawMessage
}

type OneBotMessagePartial struct {
	PostType         string           `json:"post_type"`
	MessageType      string           `json:"message_type"`
	MessageFormat    string           `json:"message_format"`
	UnDecodedMessage json.RawMessage  `json:"message"`
	MessageArray     []MessageContent `json:"-"`
	MessageString    string           `json:"-"`
	UserID           int64            `json:"user_id"`
	GroupID          int64            `json:"group_id"`
	SelfID           int64            `json:"self_id"`
	RawMessage       string           `json:"raw_message"`
	Echo             string           `json:"echo"`
	Sender           OneBotSender     `json:"sender"`
}

type OneBotSender struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Card     string `json:"card"`
	Role     string `json:"role"`
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
	case MessageFormatArray:
		if err := json.Unmarshal(oneBotMessage.Partial.UnDecodedMessage, &oneBotMessage.Partial.MessageArray); err != nil {
			return nil
		}
	case MessageFormatString:
		if err := json.Unmarshal(oneBotMessage.Partial.UnDecodedMessage, &oneBotMessage.Partial.MessageString); err != nil {
			return nil
		}
	}

	return oneBotMessage
}

func GetMessageUserID(onebotMessage *OneBotMessage) int64 {
	if onebotMessage == nil {
		return 0
	}
	if onebotMessage.Partial.Sender.UserID > 0 {
		return onebotMessage.Partial.Sender.UserID
	}
	return onebotMessage.Partial.UserID
}

func NormalizeBotID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

func (wss *WsServer) WsServerHandler() error {
	ctx, cancel := context.WithCancel(context.Background())
	readChan := make(chan WsMsg, 128)
	writeChan := make(chan WsMsg, 128)

	wss.readChan = readChan
	wss.writeChan = writeChan
	wss.apiWaiters = make(map[string]chan map[string]interface{})
	wss.initCommandDispatcher(ctx)

	go wss.readLoop(ctx, readChan)
	go wss.writeLoop(ctx, writeChan)
	go wss.refreshBotLoginInfo()
	defer wss.close(cancel, readChan, writeChan)

	for {
		mt, msg, err := wss.Conn.ReadMessage()
		if err != nil {
			return err
		}
		readChan <- WsMsg{MsgType: mt, MsgData: msg}
	}
}

func (wss *WsServer) WriteMessage(mt int, msg []byte) error {
	if wss.Conn == nil {
		return errors.New("尚未连接到 OneBot 客户端")
	}
	wss.writeChan <- WsMsg{MsgType: mt, MsgData: msg}
	return nil
}

func (wss *WsServer) AddWsClient(wsClient *WsClient) error {
	for _, c := range wss.WsClients {
		if c.Name == wsClient.Name {
			return fmt.Errorf("客户端 %s 已存在", wsClient.Name)
		}
	}
	wss.WsClients = append(wss.WsClients, wsClient)
	return nil
}

func (wss *WsServer) RemoveWsClient(name string) {
	for i, c := range wss.WsClients {
		if c.Name == name {
			wss.WsClients = append(wss.WsClients[:i], wss.WsClients[i+1:]...)
			return
		}
	}
}

func (wss *WsServer) close(cancel context.CancelFunc, readChan chan WsMsg, writeChan chan WsMsg) {
	cancel()
	conn := wss.Conn
	wss.Conn = nil
	wss.commandQueue = nil
	if conn != nil {
		conn.Close()
	}
	close(readChan)
	close(writeChan)
}

func (wss *WsServer) readLoop(ctx context.Context, readChan chan WsMsg) {
	wss.initCommandDispatcher(ctx)
	for {
		select {
		case msg, ok := <-readChan:
			if !ok {
				return
			}
			handled := false
			if msg.MsgType == websocket.TextMessage {
				if wss.dispatchAPICallback(msg.MsgData) {
					continue
				}

				wss.logDebugMessage(msg.MsgData)
				onebotMessage := ParseOneBotMessage(msg.MsgData)
				if onebotMessage != nil {
					logInfoIncomingMessage(onebotMessage)
					for i, hook := range wss.upstreamEventHooks {
						callUpstreamEventHook(i, hook, onebotMessage)
					}
					if wss.enqueueMatchedCommand(onebotMessage) {
						continue
					}
				}
				for i, handler := range wss.externalCommandHandlers {
					if handled, response := callExternalCommandHandler(i, handler, onebotMessage); handled {
						if response != nil {
							if err := wss.SendCommandResponse(response); err != nil {
								log.Printf("发送命令响应失败：%v\n", err)
							}
						}
						handled = true
						break
					}
				}
			}
			if handled {
				continue
			}

			for _, wsClient := range wss.WsClients {
				go func(wsClient *WsClient, mt int, data []byte) {
					if err := wsClient.WriteMessage(mt, data); err != nil {
						log.Printf("向 %s 转发消息失败：%v\n", wsClient.Name, err)
					}
				}(wsClient, msg.MsgType, msg.MsgData)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (wss *WsServer) dispatchAPICallback(msgData []byte) bool {
	var payload map[string]interface{}
	if err := json.Unmarshal(msgData, &payload); err != nil {
		return false
	}
	echo, _ := payload["echo"].(string)
	if echo == "" {
		return false
	}
	wss.apiWaitersMu.Lock()
	waiter, ok := wss.apiWaiters[echo]
	if ok {
		delete(wss.apiWaiters, echo)
	}
	wss.apiWaitersMu.Unlock()
	if !ok {
		return false
	}
	select {
	case waiter <- payload:
	default:
	}
	close(waiter)
	return true
}

func (wss *WsServer) logDebugMessage(msgData []byte) {
	if !CONFIG.Server.Debug {
		return
	}

	var debugMsg map[string]interface{}
	if err := json.Unmarshal(msgData, &debugMsg); err != nil {
		return
	}
	if postType, ok := debugMsg["post_type"].(string); ok {
		msgType, _ := debugMsg["message_type"].(string)
		subType, _ := debugMsg["sub_type"].(string)
		userID, _ := debugMsg["user_id"].(float64)
		groupID, _ := debugMsg["group_id"].(float64)
		log.Printf("[OneBot] Event: post_type=%s msg_type=%s sub_type=%s group=%d user=%d\n",
			postType, msgType, subType, int64(groupID), int64(userID))
		return
	}
	if echo, ok := debugMsg["echo"].(string); ok {
		log.Printf("[OneBot] ApiResp: echo=%s retcode=%v\n", echo, debugMsg["retcode"])
	}
}

func logDebugBotAppMessage(botName string, msgType int, msgData []byte) {
	if !CONFIG.Server.Debug {
		return
	}
	if !shouldLogBotAppMessage(msgType, msgData) {
		return
	}
	log.Printf("[BotApp %s] %s\n", botName, summarizeBotAppPayload(msgType, msgData))
}

func logInfoIncomingMessage(msg *OneBotMessage) {
	if msg == nil {
		return
	}
	if msg.Partial.PostType != "message" && msg.Partial.PostType != "message_sent" {
		return
	}
	raw := truncateForLog(strings.TrimSpace(msg.Partial.RawMessage), 120)
	if raw == "" {
		raw = previewMessageContents(msg.Partial.MessageArray)
	}
	switch msg.Partial.MessageType {
	case "group":
		log.Printf("[Recv][group:%d][user:%d] %s\n", msg.Partial.GroupID, GetMessageUserID(msg), raw)
	case "private":
		log.Printf("[Recv][private][user:%d] %s\n", GetMessageUserID(msg), raw)
	}
}

func shouldLogBotAppMessage(msgType int, msgData []byte) bool {
	if msgType != websocket.TextMessage {
		return false
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(msgData, &payload); err != nil {
		return true
	}
	action, _ := payload["action"].(string)
	action = strings.TrimSpace(action)
	return strings.HasPrefix(strings.ToLower(action), "send")
}

func summarizeBotAppPayload(msgType int, msgData []byte) string {
	if msgType != websocket.TextMessage {
		return fmt.Sprintf("ws_type=%d bytes=%d", msgType, len(msgData))
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(msgData, &payload); err != nil {
		return fmt.Sprintf("raw=%s", truncateForLog(string(msgData), 240))
	}

	action, _ := payload["action"].(string)
	params, _ := payload["params"].(map[string]interface{})
	if action == "" || params == nil {
		echo, _ := payload["echo"].(string)
		if echo != "" {
			return fmt.Sprintf("api_resp echo=%s retcode=%v", echo, payload["retcode"])
		}
		return fmt.Sprintf("payload=%s", truncateForLog(string(msgData), 240))
	}

	sessionType := "unknown"
	sessionID := int64(0)
	switch action {
	case "send_group_msg", "send_group_forward_msg":
		sessionType = "group"
		sessionID = toInt64(params["group_id"])
	case "send_private_msg", "send_private_forward_msg":
		sessionType = "private"
		sessionID = toInt64(params["user_id"])
	case "send_msg":
		if toInt64(params["group_id"]) > 0 || strings.TrimSpace(fmt.Sprintf("%v", params["message_type"])) == "group" {
			sessionType = "group"
			sessionID = toInt64(params["group_id"])
		} else {
			sessionType = "private"
			sessionID = toInt64(params["user_id"])
		}
	default:
		sessionType = "unknown"
	}

	return fmt.Sprintf(
		"action=%s session=%s(%d) preview=%s bytes=%d",
		action,
		sessionType,
		sessionID,
		truncateForLog(extractOutgoingMessagePreview(params["message"]), 80),
		len(msgData),
	)
}

func truncateForLog(text string, limit int) string {
	text = strings.TrimSpace(text)
	if len(text) <= limit || limit <= 0 {
		return text
	}
	return text[:limit] + "..."
}

func extractOutgoingMessagePreview(v interface{}) string {
	switch value := v.(type) {
	case string:
		return strings.TrimSpace(value)
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			segment, _ := item.(map[string]interface{})
			if segment == nil {
				continue
			}
			segType, _ := segment["type"].(string)
			data, _ := segment["data"].(map[string]interface{})
			switch segType {
			case "text":
				if text, _ := data["text"].(string); strings.TrimSpace(text) != "" {
					parts = append(parts, strings.TrimSpace(text))
				}
			default:
				if segType != "" {
					parts = append(parts, "["+segType+"]")
				}
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	default:
		return ""
	}
}

func previewMessageContents(contents []MessageContent) string {
	parts := make([]string, 0, len(contents))
	for _, item := range contents {
		switch item.Type {
		case MessageTypeText:
			if text, _ := item.Data["text"].(string); strings.TrimSpace(text) != "" {
				parts = append(parts, strings.TrimSpace(text))
			}
		default:
			if item.Type != "" {
				parts = append(parts, "["+item.Type+"]")
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func logInfoOutgoingAction(source string, payload map[string]interface{}) {
	action, _ := payload["action"].(string)
	params, _ := payload["params"].(map[string]interface{})
	if action == "" || params == nil || !strings.HasPrefix(strings.ToLower(action), "send") {
		return
	}
	sessionType := "private"
	sessionID := toInt64(params["user_id"])
	if action == "send_group_msg" || action == "send_group_forward_msg" || toInt64(params["group_id"]) > 0 || strings.TrimSpace(fmt.Sprintf("%v", params["message_type"])) == "group" {
		sessionType = "group"
		sessionID = toInt64(params["group_id"])
	}
	preview := truncateForLog(extractOutgoingMessagePreview(params["message"]), 120)
	if preview == "" {
		preview = "[" + action + "]"
	}
	if sessionType == "group" {
		log.Printf("[Send][%s][group:%d] %s\n", source, sessionID, preview)
		return
	}
	log.Printf("[Send][%s][private:%d] %s\n", source, sessionID, preview)
}

func (wss *WsServer) initCommandDispatcher(ctx context.Context) {
	if len(wss.externalCommandRoutes) == 0 || wss.commandQueue != nil {
		return
	}
	queue := make(chan externalCommandTask, 128)
	wss.commandQueue = queue
	go wss.commandWorker(ctx, queue)
}

func (wss *WsServer) commandWorker(ctx context.Context, queue chan externalCommandTask) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-queue:
			response := callExternalCommandExecutor(task.route, task.msg)
			if response == nil {
				continue
			}
			if err := wss.SendCommandResponse(response); err != nil {
				log.Printf("发送命令响应失败：route=%s err=%v\n", task.route.Name, err)
			}
		}
	}
}

func (wss *WsServer) enqueueMatchedCommand(msg *OneBotMessage) bool {
	if msg == nil || len(wss.externalCommandRoutes) == 0 {
		return false
	}
	for _, route := range wss.externalCommandRoutes {
		if !callExternalCommandMatcher(route, msg) {
			continue
		}
		if wss.commandQueue == nil {
			response := callExternalCommandExecutor(route, msg)
			if response != nil {
				if err := wss.SendCommandResponse(response); err != nil {
					log.Printf("发送命令响应失败：route=%s err=%v\n", route.Name, err)
				}
			}
			return true
		}
		select {
		case wss.commandQueue <- externalCommandTask{route: route, msg: msg}:
			return true
		default:
			log.Printf("命令队列已满，丢弃命令：route=%s raw=%s\n", route.Name, truncateForLog(msg.Partial.RawMessage, 120))
			response := buildBusyReply(msg, "命令处理繁忙，请稍后重试")
			if err := wss.SendCommandResponse(response); err != nil {
				log.Printf("发送繁忙提示失败：route=%s err=%v\n", route.Name, err)
			}
			return true
		}
	}
	return false
}

func buildBusyReply(msg *OneBotMessage, text string) map[string]interface{} {
	params := map[string]interface{}{"message": text}
	if msg != nil && msg.Partial.MessageType == "group" {
		params["group_id"] = msg.Partial.GroupID
		return map[string]interface{}{"action": "send_group_msg", "params": params}
	}
	if msg != nil {
		params["user_id"] = GetMessageUserID(msg)
	}
	return map[string]interface{}{"action": "send_private_msg", "params": params}
}

func logPluginPanic(kind string, index int, detail string, recovered interface{}) {
	name := fmt.Sprintf("%s#%d", kind, index)
	if strings.TrimSpace(detail) != "" {
		name += "(" + detail + ")"
	}
	log.Printf("[PANIC][%s] %v\n%s", name, recovered, debug.Stack())
}

func callExternalCommandHandler(index int, handler ExternalCommandHandler, msg *OneBotMessage) (handled bool, response map[string]interface{}) {
	defer func() {
		if recovered := recover(); recovered != nil {
			handled = false
			response = nil
			logPluginPanic("command-handler", index, "", recovered)
		}
	}()
	return handler(msg)
}

func callExternalCommandMatcher(route ExternalCommandRoute, msg *OneBotMessage) (matched bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			matched = false
			logPluginPanic("command-match", 0, route.Name, recovered)
		}
	}()
	if route.Match == nil {
		return false
	}
	return route.Match(msg)
}

func callExternalCommandExecutor(route ExternalCommandRoute, msg *OneBotMessage) (response map[string]interface{}) {
	defer func() {
		if recovered := recover(); recovered != nil {
			response = nil
			logPluginPanic("command-exec", 0, route.Name, recovered)
		}
	}()
	if route.Execute == nil {
		return nil
	}
	return route.Execute(msg)
}

func callUpstreamEventHook(index int, hook UpstreamEventHook, msg *OneBotMessage) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logPluginPanic("upstream-hook", index, "", recovered)
		}
	}()
	hook(msg)
}

func callBotActionHook(index int, botName string, hook BotActionHook, msgType int, msgData []byte) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logPluginPanic("bot-action-hook", index, botName, recovered)
		}
	}()
	hook(botName, msgType, msgData)
}

func callInternalSendHook(index int, hook InternalSendHook, response map[string]interface{}) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logPluginPanic("internal-send-hook", index, "", recovered)
		}
	}()
	hook(response)
}

func safeFilterMessage(clientName string, filter MessageFilter, msg *OneBotMessage) (allowed bool) {
	if filter == nil {
		return true
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			allowed = true
			log.Printf("[PANIC][message-filter:%s] %v\n%s", clientName, recovered, debug.Stack())
		}
	}()
	return filter.Filter(msg)
}

func (wss *WsServer) writeLoop(ctx context.Context, writeChan chan WsMsg) {
	for {
		select {
		case msg, ok := <-writeChan:
			if !ok {
				return
			}
			data := msg.MsgData
			if msg.MsgType == websocket.TextMessage {
				data = ConvertFileToBase64(CONFIG.Server, data)
				data = ConvertFileToURL(CONFIG.Server, CONFIG.Server.AccessToken, data)
			}
			if err := wss.Conn.WriteMessage(msg.MsgType, data); err != nil {
				log.Println("写入 OneBot 客户端失败：", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (wss *WsServer) SendCommandResponse(response map[string]interface{}) error {
	if wss.Conn == nil {
		return errors.New("尚未连接到 OneBot 客户端")
	}
	for i, hook := range wss.internalSendHooks {
		callInternalSendHook(i, hook, response)
	}
	responseData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("序列化命令响应失败: %v", err)
	}
	if err := wss.WriteMessage(websocket.TextMessage, responseData); err != nil {
		return fmt.Errorf("发送命令响应失败: %v", err)
	}
	logInfoOutgoingAction("filter", response)
	if CONFIG.Server.Debug {
		log.Printf("已发送命令响应到 OneBot 客户端：%s\n", string(responseData))
	}
	return nil
}

func (wss *WsServer) nextEcho(prefix string) string {
	seq := atomic.AddUint64(&wss.echoSeq, 1)
	return fmt.Sprintf("%s-%d", prefix, seq)
}

func (wss *WsServer) CallAPI(action string, params map[string]interface{}, timeout time.Duration) (map[string]interface{}, error) {
	if wss.Conn == nil {
		return nil, errors.New("尚未连接到 OneBot 客户端")
	}
	echo := wss.nextEcho("onebotfilter")
	payload := map[string]interface{}{
		"action": action,
		"params": params,
		"echo":   echo,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化 API 请求失败: %w", err)
	}
	waiter := make(chan map[string]interface{}, 1)
	wss.apiWaitersMu.Lock()
	wss.apiWaiters[echo] = waiter
	wss.apiWaitersMu.Unlock()
	if err := wss.WriteMessage(websocket.TextMessage, data); err != nil {
		wss.apiWaitersMu.Lock()
		delete(wss.apiWaiters, echo)
		wss.apiWaitersMu.Unlock()
		return nil, err
	}
	select {
	case resp := <-waiter:
		return resp, nil
	case <-time.After(timeout):
		wss.apiWaitersMu.Lock()
		delete(wss.apiWaiters, echo)
		wss.apiWaitersMu.Unlock()
		return nil, fmt.Errorf("调用 %s 超时", action)
	}
}

func (wss *WsServer) refreshBotLoginInfo() {
	resp, err := wss.CallAPI("get_login_info", map[string]interface{}{}, 5*time.Second)
	if err != nil {
		if CONFIG.Server.Debug {
			log.Printf("获取 bot 登录信息失败: %v\n", err)
		}
		return
	}
	wss.applyLoginInfo(extractLoginInfo(resp))
}

func extractLoginInfo(resp map[string]interface{}) (string, string) {
	data, _ := resp["data"].(map[string]interface{})
	nickname, _ := data["nickname"].(string)
	return int64ToBotID(toInt64(data["user_id"])), strings.TrimSpace(nickname)
}

func (wss *WsServer) applyLoginInfo(loginBotID, nickname string) {
	if nickname != "" {
		SetBotNickname(nickname)
	}
	if NormalizeBotID(CONFIG.Server.BotID) != "" || loginBotID == "" {
		return
	}
	if wss.BotID != "" && wss.BotID != loginBotID {
		log.Printf("检测到 get_login_info 返回的 Bot ID 与当前连接标识不一致 (%s -> %s)，正在重置下游连接...\n", wss.BotID, loginBotID)
		wss.DisconnectAllClients()
	}
	wss.BotID = loginBotID
}

func int64ToBotID(value int64) string {
	if value <= 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

func toInt64(value interface{}) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case float32:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	default:
		return 0
	}
}

func (wss *WsServer) DisconnectAllClients() {
	clients := make([]*WsClient, len(wss.WsClients))
	copy(clients, wss.WsClients)
	for _, client := range clients {
		if client.conn != nil {
			client.conn.Close()
		}
	}
}

func buildBotAppHeaders(botID, accessToken, userAgent string) http.Header {
	header := http.Header{}
	if botID != "" {
		header["X-Self-ID"] = []string{botID}
	}
	if accessToken != "" {
		header["Authorization"] = []string{fmt.Sprintf("Bearer %s", accessToken)}
	}
	if userAgent != "" {
		header["User-Agent"] = []string{userAgent}
	}
	header["X-Client-Role"] = []string{"Universal"}
	return header
}

func WsClientHandler(wss *WsServer, cfg BotAppsConfig, filter MessageFilter) {
	if err := cfg.Check(); err != nil {
		log.Printf("%s 的配置有问题：%v\n", cfg.Name, err)
		return
	}

	for {
		for wss.BotID == "" && CONFIG.Server.BotID == "" {
			time.Sleep(time.Duration(CONFIG.Server.SleepTime) * time.Second)
		}

		botID := wss.BotID
		if botID == "" {
			botID = CONFIG.Server.BotID
		}
		header := buildBotAppHeaders(botID, cfg.AccessToken, CONFIG.Server.UserAgent)

		dialer := &websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: 45 * time.Second,
			ReadBufferSize:   CONFIG.Server.BufferSize,
			WriteBufferSize:  CONFIG.Server.BufferSize,
		}
		conn, _, err := dialer.Dial(cfg.URI, header)
		if err != nil {
			log.Printf("连接 %s 失败：%v\n", cfg.Name, err)
			time.Sleep(time.Duration(CONFIG.Server.SleepTime) * time.Second)
			continue
		}

		client := &WsClient{
			Name:      cfg.Name,
			URI:       cfg.URI,
			Access:    cfg.AccessToken,
			conn:      conn,
			filter:    filter,
			readChan:  make(chan WsMsg, 128),
			writeChan: make(chan WsMsg, 128),
		}
		if err := wss.AddWsClient(client); err != nil {
			log.Printf("注册客户端 %s 失败：%v\n", cfg.Name, err)
			client.conn.Close()
			time.Sleep(time.Duration(CONFIG.Server.SleepTime) * time.Second)
			continue
		}

		ctx, cancel := context.WithCancel(context.Background())
		go client.readLoop(ctx, wss)
		go client.writeLoop(ctx)

		if filter != nil {
			log.Printf("已连接到 %s，加载的过滤器：%s\n", cfg.Name, filter.String())
		} else {
			log.Printf("已连接到 %s\n", cfg.Name)
		}

		for {
			mt, msg, err := client.conn.ReadMessage()
			if err != nil {
				client.conn.Close()
				wss.RemoveWsClient(client.Name)
				time.Sleep(5 * time.Second)
				break
			}
			client.readChan <- WsMsg{MsgType: mt, MsgData: msg}
		}
		client.close(cancel)
	}
}

func (wc *WsClient) WriteMessage(mt int, msg []byte) error {
	if wc.conn == nil {
		return errors.New("尚未连接到 bot 应用端")
	}
	wc.writeChan <- WsMsg{MsgType: mt, MsgData: msg}
	return nil
}

func (wc *WsClient) close(cancel context.CancelFunc) {
	cancel()
	if wc.conn != nil {
		wc.conn.Close()
	}
	close(wc.readChan)
	close(wc.writeChan)
	wc.conn = nil
}

func (wc *WsClient) readLoop(ctx context.Context, wss *WsServer) {
	for {
		select {
		case msg, ok := <-wc.readChan:
			if !ok {
				return
			}
			logDebugBotAppMessage(wc.Name, msg.MsgType, msg.MsgData)
			if msg.MsgType == websocket.TextMessage {
				var payload map[string]interface{}
				if err := json.Unmarshal(msg.MsgData, &payload); err == nil {
					logInfoOutgoingAction(wc.Name, payload)
				}
			}
			for i, hook := range wss.botActionHooks {
				callBotActionHook(i, wc.Name, hook, msg.MsgType, msg.MsgData)
			}
			if err := wss.WriteMessage(msg.MsgType, msg.MsgData); err != nil {
				log.Println("写入 OneBot 客户端失败：", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (wc *WsClient) writeLoop(ctx context.Context) {
	for {
		select {
		case msg, ok := <-wc.writeChan:
			if !ok {
				return
			}
			if msg.MsgType == websocket.TextMessage {
				onebotMessage := ParseOneBotMessage(msg.MsgData)
				if onebotMessage == nil {
					if err := wc.conn.WriteMessage(msg.MsgType, msg.MsgData); err != nil {
						log.Printf("向 %s 转发原始文本消息失败：%v\n", wc.Name, err)
					}
					continue
				}
				if onebotMessage.Partial.RawMessage != "" {
					if safeFilterMessage(wc.Name, wc.filter, onebotMessage) {
						if err := wc.conn.WriteJSON(onebotMessage.Intact); err != nil {
							log.Printf("向 %s 发送过滤后的消息失败：%v\n", wc.Name, err)
						}
					} else if CONFIG.Server.Debug {
						log.Printf("%s：消息被过滤器阻止：%s\n", wc.Name, onebotMessage.Partial.RawMessage)
					}
					continue
				}
			}
			if err := wc.conn.WriteMessage(msg.MsgType, msg.MsgData); err != nil {
				log.Printf("向 %s 转发消息失败：%v\n", wc.Name, err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func ConvertFileToBase64(cfg ServerConfig, msgData []byte) []byte {
	fileCfg := cfg.FileServer
	if !fileCfg.Base64Enabled || fileCfg.Root == "" {
		return msgData
	}

	root := strings.TrimRight(fileCfg.Root, "/")
	msg := string(msgData)
	cache := make(map[string]string)

	for strings.Contains(msg, "file:////") {
		msg = strings.ReplaceAll(msg, "file:////", "file:///")
	}

	msg, _ = replaceQuotedFileValue(msg, `"file":"`, root, cache, fileCfg.Base64MaxSize)
	msg, _ = replaceQuotedFileValue(msg, `"file": "`, root, cache, fileCfg.Base64MaxSize)
	msg, _ = replaceFileURIValue(msg, root, cache, fileCfg.Base64MaxSize)
	msg, _ = replaceCQFileValue(msg, root, cache, fileCfg.Base64MaxSize)

	return []byte(msg)
}

func ConvertFileToURL(cfg ServerConfig, accessToken string, msgData []byte) []byte {
	fileCfg := cfg.FileServer
	if !fileCfg.Enabled || fileCfg.Root == "" || fileCfg.PublicURL == "" {
		return msgData
	}

	root := strings.TrimRight(fileCfg.Root, "/")
	publicURL := strings.TrimRight(fileCfg.PublicURL, "/")

	var httpPrefix string
	if accessToken != "" {
		httpPrefix = publicURL + "/files/" + accessToken
	} else {
		httpPrefix = publicURL + "/files"
	}

	msg := string(msgData)

	for strings.Contains(msg, "file:////") {
		msg = strings.ReplaceAll(msg, "file:////", "file:///")
	}

	oldFileURL := "file://" + root
	msg = strings.ReplaceAll(msg, oldFileURL, httpPrefix)

	oldJSONPath := `"file":"` + root
	newJSONPath := `"file":"` + httpPrefix
	msg = strings.ReplaceAll(msg, oldJSONPath, newJSONPath)

	oldJSONPathWithSpace := `"file": "` + root
	newJSONPathWithSpace := `"file": "` + httpPrefix
	msg = strings.ReplaceAll(msg, oldJSONPathWithSpace, newJSONPathWithSpace)

	oldCQPath := "file=" + root
	newCQPath := "file=" + httpPrefix
	msg = strings.ReplaceAll(msg, oldCQPath, newCQPath)

	return []byte(msg)
}

func replaceQuotedFileValue(msg string, marker string, root string, cache map[string]string, maxSize int64) (string, bool) {
	searchIndex := 0
	changed := false

	for {
		startOffset := strings.Index(msg[searchIndex:], marker)
		if startOffset < 0 {
			break
		}

		start := searchIndex + startOffset
		valueStart := start + len(marker)
		valueEndOffset := strings.Index(msg[valueStart:], `"`)
		if valueEndOffset < 0 {
			break
		}

		valueEnd := valueStart + valueEndOffset
		converted, ok := encodeFileReference(msg[valueStart:valueEnd], root, cache, maxSize)
		if !ok {
			searchIndex = valueEnd
			continue
		}

		msg = msg[:valueStart] + converted + msg[valueEnd:]
		searchIndex = valueStart + len(converted)
		changed = true
	}

	return msg, changed
}

func replaceFileURIValue(msg string, root string, cache map[string]string, maxSize int64) (string, bool) {
	const marker = "file://"
	searchIndex := 0
	changed := false

	for {
		startOffset := strings.Index(msg[searchIndex:], marker)
		if startOffset < 0 {
			break
		}

		start := searchIndex + startOffset
		end := findValueEnd(msg, start, "\"' \r\n\t,]}")
		converted, ok := encodeFileReference(msg[start:end], root, cache, maxSize)
		if !ok {
			searchIndex = end
			continue
		}

		msg = msg[:start] + converted + msg[end:]
		searchIndex = start + len(converted)
		changed = true
	}

	return msg, changed
}

func replaceCQFileValue(msg string, root string, cache map[string]string, maxSize int64) (string, bool) {
	const marker = "file="
	searchIndex := 0
	changed := false

	for {
		startOffset := strings.Index(msg[searchIndex:], marker)
		if startOffset < 0 {
			break
		}

		start := searchIndex + startOffset
		valueStart := start + len(marker)
		end := findValueEnd(msg, valueStart, ",] \r\n\t")
		converted, ok := encodeFileReference(msg[valueStart:end], root, cache, maxSize)
		if !ok {
			searchIndex = end
			continue
		}

		msg = msg[:valueStart] + converted + msg[end:]
		searchIndex = valueStart + len(converted)
		changed = true
	}

	return msg, changed
}

func findValueEnd(msg string, start int, terminators string) int {
	for i := start; i < len(msg); i++ {
		if strings.ContainsRune(terminators, rune(msg[i])) {
			return i
		}
	}
	return len(msg)
}

func encodeFileReference(value string, root string, cache map[string]string, maxSize int64) (string, bool) {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "base64://") {
		return "", false
	}

	pathValue := strings.TrimPrefix(value, "file://")
	if pathValue == "" {
		return "", false
	}

	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(pathValue)
	if !isSubPath(cleanRoot, cleanPath) {
		return "", false
	}

	if cached, ok := cache[cleanPath]; ok {
		return cached, true
	}

	if maxSize > 0 {
		fileInfo, err := os.Stat(cleanPath)
		if err != nil || fileInfo.Size() > maxSize {
			return "", false
		}
	}

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", false
	}

	converted := "base64://" + base64.StdEncoding.EncodeToString(content)
	cache[cleanPath] = converted
	return converted, true
}

func isSubPath(root string, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && rel != "..")
}
