package onebotfilter

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type WsClient struct {
	Name      string
	conn      *websocket.Conn
	filter    *Filter
	readChan  chan WsMsg
	writeChan chan WsMsg
}

func WsClientHandler(wss *WsServer, cfg BotAppsConfig) {
	if err := cfg.Check(); err != nil {
		log.Printf("%s 的配置有问题：%v\n", cfg.Name, err)
		return
	}

	filter := (&Filter{Name: cfg.Name}).Compile(cfg)
	AddFilter(filter)
	defer RemoveFilter(filter.Name)

	for {
		for wss.BotId == "" && CONFIG.Server.BotId == "" {
			log.Printf("等待 OneBot 客户端连接以获取 Bot ID...\n")
			time.Sleep(time.Duration(CONFIG.Server.SleepTime) * time.Second)
		}

		header := http.Header{}
		botID := wss.BotId
		if botID == "" {
			botID = CONFIG.Server.BotId
		}
		header.Set("x-self-id", botID)
		header.Set("authorization", fmt.Sprintf("Bearer %s", cfg.AccessToken))
		header.Set("user-agent", CONFIG.Server.UserAgent)
		header.Set("x-client-role", "Universal")

		log.Printf("正在连接 %s (Bot ID: %s)\n", cfg.Name, botID)

		dialer := &websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: 45 * time.Second,
			ReadBufferSize:   CONFIG.Server.BufferSize,
			WriteBufferSize:  CONFIG.Server.BufferSize,
		}
		conn, _, err := dialer.Dial(cfg.Uri, header)
		if err != nil {
			log.Printf("连接 %s 失败：%v\n", cfg.Name, err)
			time.Sleep(time.Duration(CONFIG.Server.SleepTime) * time.Second)
			continue
		}

		client := &WsClient{
			Name:      cfg.Name,
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

		log.Printf("已连接到 %s，加载的过滤器：%s\n", cfg.Name, filter.String())

		for {
			mt, msg, err := client.conn.ReadMessage()
			if err != nil {
				log.Printf("从 %s 读取消息失败：%v\n", cfg.Name, err)
				client.conn.Close()
				wss.RemoveWsClient(client.Name)
				time.Sleep(5 * time.Second)
				break
			}
			client.readChan <- WsMsg{mt, msg}
		}

		client.close(cancel)
	}
}

func (wc *WsClient) WriteMessage(mt int, msg []byte) error {
	if wc.conn == nil {
		return errors.New("尚未连接到 bot 应用端")
	}
	wc.writeChan <- WsMsg{mt, msg}
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
		case msg := <-wc.readChan:
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
		case msg := <-wc.writeChan:
			if msg.MsgType == websocket.TextMessage {
				onebotMessage := ParseOneBotMessage(msg.MsgData)
				if onebotMessage == nil {
					if err := wc.conn.WriteMessage(msg.MsgType, msg.MsgData); err != nil {
						log.Printf("向 %s 转发原始文本消息失败：%v\n", wc.Name, err)
					}
					continue
				}

				if onebotMessage.Partial.RawMessage != "" {
					if wc.filter.Filter(onebotMessage) {
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
