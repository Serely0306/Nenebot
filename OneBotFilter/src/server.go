package onebotfilter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/gorilla/websocket"
)

type WsServer struct {
	Conn      *websocket.Conn
	WsClients []*WsClient
	BotId     string     // 从OneBot客户端连接中自动获取的bot ID
	readChan  chan WsMsg //从OneBot客户端读取到的消息
	writeChan chan WsMsg //写入到OneBot客户端的消息
	// mutex     sync.Mutex
}

// 处理与OneBot客户端的连接
func (wss *WsServer) WsServerHandler() error {
	ctx, ctxCancel := context.WithCancel(context.Background())
	wss.readChan = make(chan WsMsg, 128)
	wss.writeChan = make(chan WsMsg, 128)
	go wss.readLoop(ctx)       //开启读取OneBot客户端消息协程
	go wss.writeLoop(ctx)      //开启写入OneBot客户端消息携程
	defer wss.close(ctxCancel) //注册关闭方法
	for {
		mt, msg, err := wss.Conn.ReadMessage()
		if err != nil {
			return err
		}
		wss.readChan <- WsMsg{mt, msg}
	}
	// return errors.New("读取消息循环已结束")
}

// 向OneBot客户端写入消息
func (wss *WsServer) WriteMessage(mt int, msg []byte) error {
	if wss.Conn == nil {
		return errors.New("没有连接到OneBot客户端")
	}
	wss.writeChan <- WsMsg{mt, msg}
	return nil
}

// 添加bot应用端
func (wss *WsServer) AddWsClient(wsClient *WsClient) error {
	// wss.mutex.Lock()
	// defer wss.mutex.Unlock()
	for _, c := range wss.WsClients {
		if c.Name == wsClient.Name {
			return fmt.Errorf("已经连接过%s", wsClient.Name)
		}
	}
	wss.WsClients = append(wss.WsClients, wsClient)
	return nil
}

// 删除bot应用端
func (wss *WsServer) RemoveWsClient(name string) {
	// wss.mutex.Lock()
	// defer wss.mutex.Unlock()
	for i, c := range wss.WsClients {
		if c.Name == name {
			wss.WsClients = append(wss.WsClients[:i], wss.WsClients[i+1:]...) //从列表中删除
			return
		}
	}
}

// 关闭连接
func (wss *WsServer) close(ctxCancel context.CancelFunc) {
	ctxCancel()
	conn := wss.Conn
	wss.Conn = nil // 先置nil，让handleLocal能尽快接受新连接
	if conn != nil {
		conn.Close()
	}
	close(wss.readChan)
	close(wss.writeChan)
}

func (wss *WsServer) readLoop(ctx context.Context) {
	for {
		select {
		case msg := <-wss.readChan:
			if msg.MsgType == websocket.TextMessage {
				// 简单的日志记录用于调试
				var debugMsg map[string]interface{}
				if err := json.Unmarshal(msg.MsgData, &debugMsg); err == nil {
					if postType, ok := debugMsg["post_type"].(string); ok {
						// 是事件
						msgType, _ := debugMsg["message_type"].(string)
						subType, _ := debugMsg["sub_type"].(string)
						userId, _ := debugMsg["user_id"].(float64) // json number -> float64
						groupId, _ := debugMsg["group_id"].(float64)
						log.Printf("[OneBot] Event: post_type=%s msg_type=%s sub_type=%s group=%d user=%d\n",
							postType, msgType, subType, int64(groupId), int64(userId))
					} else if echo, ok := debugMsg["echo"].(string); ok {
						// 是API响应
						log.Printf("[OneBot] ApiResp: echo=%s retcode=%v\n", echo, debugMsg["retcode"])
					}
				}

				onebotMessage := ParseOneBotMessage(msg.MsgData)
				if onebotMessage != nil && onebotMessage.Partial.MessageType == GROUP {
					// 尝试作为命令处理
					processed := false

					// 检查是否是 /启用 或 /禁用 命令
					var messageText string
					if onebotMessage.Partial.MessageFormat == MESSAGE_FORMAT_ARRAY {
						for _, msg := range onebotMessage.Partial.MessageArray {
							if msg.Type == MESSAGE_TYPE_TEXT {
								messageText = strings.TrimSpace(msg.Data["text"].(string))
								break
							}
						}
					} else {
						messageText = strings.TrimSpace(onebotMessage.Partial.MessageString)
					}

					// 解析命令
					parts := strings.Fields(messageText)
					if len(parts) == 2 && (parts[0] == "/启用" || parts[0] == "/禁用") {
						command := parts[0]
						botName := parts[1]
						groupId := onebotMessage.Partial.GroupId

						log.Printf("收到命令: %s %s, 群: %d\n", command, botName, groupId)

						// 查找对应的过滤器
						for _, filter := range ALL_FILTERS {
							if filter.Name == botName {
								var responseMsg string

								if command == "/禁用" {
									if filter.AddToBlacklist(GROUP, groupId) {
										responseMsg = fmt.Sprintf("%s禁用成功", botName)
									} else {
										responseMsg = fmt.Sprintf("%s禁用失败", botName)
									}
								} else { // /启用
									if filter.RemoveFromBlacklist(GROUP, groupId) {
										responseMsg = fmt.Sprintf("%s启用成功", botName)
									} else {
										responseMsg = fmt.Sprintf("%s启用失败", botName)
									}
								}

								// 创建回复消息
								reply := map[string]interface{}{
									"action": "send_group_msg",
									"params": map[string]interface{}{
										"group_id": groupId,
										"message":  responseMsg,
									},
								}

								// 发送回复给 OneBot 客户端
								replyJSON, _ := json.Marshal(reply)
								wss.Conn.WriteMessage(websocket.TextMessage, replyJSON)
								log.Printf("已发送命令回复: %s\n", responseMsg)

								processed = true
								break
							}
						}
					}

					// 如果是命令且已处理，不转发给bot应用端
					if processed {
						continue
					}
				}
			}

			// 非命令消息，正常转发
			for _, wsClient := range wss.WsClients {
				go func(wsClient *WsClient, mt int, msg []byte) {
					if err := wsClient.WriteMessage(mt, msg); err != nil {
						log.Printf("向 %s 发送消息出错：%v\n", wsClient.Name, err)
					}
				}(wsClient, msg.MsgType, msg.MsgData)
			}
		case <-ctx.Done():
			return
		}
	}
}

// 处理写入OneBot客户端的消息
func (wss *WsServer) writeLoop(ctx context.Context) {
	for {
		select {
		case msg := <-wss.writeChan:
			data := msg.MsgData
			// 对文本消息进行 file:// -> http:// 转换
			if msg.MsgType == 1 { // websocket.TextMessage
				data = ConvertFileToURL(data)
			}
			if err := wss.Conn.WriteMessage(msg.MsgType, data); err != nil {
				log.Println("写入到OneBot客户端出错：", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (wss *WsServer) SendCommandResponse(response map[string]interface{}) error {
	if wss.Conn == nil {
		return errors.New("没有连接到OneBot客户端")
	}

	responseData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("序列化响应失败: %v", err)
	}

	if err := wss.Conn.WriteMessage(websocket.TextMessage, responseData); err != nil {
		return fmt.Errorf("发送响应失败: %v", err)
	}

	if CONFIG.Server.Debug {
		log.Printf("已发送命令响应到 OneBot 客户端: %s\n", string(responseData))
	}

	return nil
}

// 强制断开所有bot客户端连接（触发重连）
// 当NapCat的bot ID变化时调用，让bot客户端重新连接以更新Header
func (wss *WsServer) DisconnectAllClients() {
	log.Println("正在断开所有下游Bot客户端连接以重置状态...")
	// 复制一份列表以避免并发修改问题
	clients := make([]*WsClient, len(wss.WsClients))
	copy(clients, wss.WsClients)

	for _, client := range clients {
		log.Printf("强制断开: %s\n", client.Name)
		if client.conn != nil {
			client.conn.Close()
		}
	}
}
