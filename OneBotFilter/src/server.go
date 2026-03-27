package onebotfilter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type WsServer struct {
	Conn         *websocket.Conn
	WsClients    []*WsClient
	BotId        string
	readChan     chan WsMsg
	writeChan    chan WsMsg
	apiWaiters   map[string]chan map[string]interface{}
	apiWaitersMu sync.Mutex
	echoSeq      uint64
}

func (wss *WsServer) WsServerHandler() error {
	ctx, cancel := context.WithCancel(context.Background())
	readChan := make(chan WsMsg, 128)
	writeChan := make(chan WsMsg, 128)

	wss.readChan = readChan
	wss.writeChan = writeChan
	wss.apiWaiters = make(map[string]chan map[string]interface{})

	go wss.readLoop(ctx, readChan)
	go wss.writeLoop(ctx, writeChan)
	go wss.refreshBotLoginInfo()
	defer wss.close(cancel, readChan, writeChan)

	for {
		mt, msg, err := wss.Conn.ReadMessage()
		if err != nil {
			return err
		}
		readChan <- WsMsg{mt, msg}
	}
}

func (wss *WsServer) WriteMessage(mt int, msg []byte) error {
	if wss.Conn == nil {
		return errors.New("尚未连接到 OneBot 客户端")
	}
	wss.writeChan <- WsMsg{mt, msg}
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
	if conn != nil {
		conn.Close()
	}
	close(readChan)
	close(writeChan)
}

func (wss *WsServer) readLoop(ctx context.Context, readChan chan WsMsg) {
	for {
		select {
		case msg := <-readChan:
			if msg.MsgType == websocket.TextMessage {
				if wss.dispatchAPICallback(msg.MsgData) {
					continue
				}

				wss.logDebugMessage(msg.MsgData)

				onebotMessage := ParseOneBotMessage(msg.MsgData)
				if handled, response := handleControlCommand(onebotMessage); handled {
					if response != nil {
						if err := wss.SendCommandResponse(response); err != nil {
							log.Printf("发送命令响应失败：%v\n", err)
						}
					}
					continue
				}
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

func (wss *WsServer) writeLoop(ctx context.Context, writeChan chan WsMsg) {
	for {
		select {
		case msg := <-writeChan:
			data := msg.MsgData
			if msg.MsgType == websocket.TextMessage {
				data = ConvertFileToBase64(data)
				data = ConvertFileToURL(data)
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

	responseData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("序列化命令响应失败: %v", err)
	}

	if err := wss.WriteMessage(websocket.TextMessage, responseData); err != nil {
		return fmt.Errorf("发送命令响应失败: %v", err)
	}

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
		log.Printf("获取 bot 登录信息失败: %v\n", err)
		return
	}

	data, _ := resp["data"].(map[string]interface{})
	nickname, _ := data["nickname"].(string)
	if strings.TrimSpace(nickname) == "" {
		return
	}

	SetBotNickname(strings.TrimSpace(nickname))
	log.Printf("已获取 bot 昵称: %s\n", nickname)
}

func (wss *WsServer) DisconnectAllClients() {
	log.Println("正在断开所有下游 bot 客户端连接以重置状态..")

	clients := make([]*WsClient, len(wss.WsClients))
	copy(clients, wss.WsClients)

	for _, client := range clients {
		log.Printf("强制断开：%s\n", client.Name)
		if client.conn != nil {
			client.conn.Close()
		}
	}
}
